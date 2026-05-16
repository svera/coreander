package metadata

import (
	"archive/zip"
	"fmt"
	"html/template"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/kovidgoyal/imaging"
	"github.com/microcosm-cc/bluemonday"
	"github.com/pirmd/epub"
	"github.com/rickb777/date/v2"
	"github.com/svera/coreander/v4/internal/precisiondate"
)

type EpubReader struct {
	GetMetadataFromFile func(path string) (*epub.Information, error)
	GetPackageFromFile  func(path string) (*epub.PackageDocument, error)
}

func NewEpubReader() EpubReader {
	return EpubReader{
		GetMetadataFromFile: epub.GetMetadataFromFile,
		GetPackageFromFile:  epub.GetPackageFromFile,
	}
}

func (e EpubReader) Metadata(filename string) (Metadata, error) {
	book, err := epub.Open(filename)
	if err != nil {
		return Metadata{}, err
	}
	defer book.Close()

	info, err := book.Information()
	if err != nil {
		return Metadata{}, err
	}
	bk, err := BuildEpubMetadataFields(info, filename)
	if err != nil {
		return Metadata{}, err
	}
	opf, err := book.Package()
	if err != nil {
		log.Printf("Cannot load package for illustrations/words in %s: %s\n", filename, err)
		return bk, nil
	}
	illustrations, err := e.illustrationsWithZip(book.ReadCloser, opf, 0.25)
	if err != nil {
		log.Printf("Cannot count illustrations in %s: %s\n", filename, err)
	}
	bk.Illustrations = illustrations
	w, err := wordsFromZip(book.ReadCloser)
	if err != nil {
		log.Printf("Cannot count words in %s: %s\n", filename, err)
	}
	bk.Words = float64(w)
	return bk, nil
}

// BuildEpubMetadataFields maps pirmd/epub Information into Metadata (title, authors, dates, etc.).
// It does not open the EPUB: Illustrations and Words are left at zero unless set elsewhere.
// EpubReader.Metadata uses this then fills illustrations and word count from the package/zip.
func BuildEpubMetadataFields(meta *epub.Information, filename string) (Metadata, error) {
	title := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	if len(meta.Title) > 0 && len(meta.Title[0]) > 0 {
		title = meta.Title[0]
	}
	var authors []string
	var illustrators []string
	for _, creator := range meta.Creator {
		classifyEpubPerson(creator, true, &authors, &illustrators)
	}
	for _, contributor := range meta.Contributor {
		classifyEpubPerson(contributor, false, &authors, &illustrators)
	}
	if len(authors) == 0 {
		authors = []string{""}
	}

	var subjects []string
	for _, subject := range meta.Subject {
		subject = strings.TrimSpace(subject)
		if subject == "" {
			continue
		}
		names := strings.FieldsFunc(subject, func(r rune) bool {
			return r == ',' || r == ';'
		})
		for _, name := range names {
			if name = strings.TrimSpace(name); name != "" {
				subjects = append(subjects, name)
			}
		}
	}

	description := ""
	if len(meta.Description) > 0 {
		description = SanitizeDescription(meta.Description[0])
	}

	lang := ""
	if len(meta.Language) > 0 {
		lang = meta.Language[0]
	}

	publication := precisiondate.PrecisionDate{Precision: precisiondate.PrecisionDay}
	var err error
	for _, currentDate := range meta.Date {
		if currentDate.Event == "publication" || currentDate.Event == "" {
			if publication.Date, err = date.ParseISO(currentDate.Stamp); err != nil {
				publication.Precision = precisiondate.PrecisionYear
				publication.Date, _ = date.Parse("2006", currentDate.Stamp)
			}
			break
		}
	}

	seriesIndex, _ := strconv.ParseFloat(meta.SeriesIndex, 64)

	return Metadata{
		Title:        title,
		Authors:      authors,
		Illustrators: illustrators,
		Description:  template.HTML(description),
		Language:     lang,
		Publication:  publication,
		Series:       meta.Series,
		SeriesIndex:  seriesIndex,
		Format:       "EPUB",
		Subjects:     subjects,
	}, nil
}

func opfBaseDir(r *zip.ReadCloser) string {
	opfPath := findOpfPath(r)
	opfBaseDir := ""
	if opfPath != "" {
		opfBaseDir = path.Dir(opfPath)
		if opfBaseDir == "." {
			opfBaseDir = ""
		}
	}
	return opfBaseDir
}

func (e EpubReader) coverFileNameFromOPF(opf *epub.PackageDocument, r *zip.ReadCloser) (string, error) {
	if opf == nil {
		return "", nil
	}

	opfBaseDir := opfBaseDir(r)

	coverFileName := selectCoverFileName(opf)
	if coverFileName == "" {
		return "", nil
	}

	coverFileName = resolveHref(coverFileName, opfBaseDir)
	if isMarkupFile(coverFileName) {
		if resolvedCover, err := findCoverImageInMarkup(r, coverFileName); err == nil && resolvedCover != "" {
			coverFileName = resolvedCover
		}
	}
	return coverFileName, nil
}

// Cover parses the document looking for a cover image and returns it
func (e EpubReader) Cover(documentFullPath string, coverMaxWidth int) ([]byte, error) {
	var cover []byte

	book, err := epub.Open(documentFullPath)
	if err != nil {
		return nil, err
	}
	defer book.Close()

	opf, err := book.Package()
	if err != nil {
		return nil, err
	}

	coverFileName, err := e.coverFileNameFromOPF(opf, book.ReadCloser)
	if err != nil {
		return nil, err
	}

	cover, err = extractCover(book.ReadCloser, coverFileName, opfBaseDir(book.ReadCloser), coverMaxWidth)
	if err != nil {
		return nil, err
	}
	return cover, nil
}

// illustrationsWithZip counts images in the EPUB at least minMegapixels megapixels (excluding the cover)
// using an already-open zip and parsed package document.
func (e EpubReader) illustrationsWithZip(r *zip.ReadCloser, opf *epub.PackageDocument, minMegapixels float64) (int, error) {
	if opf == nil || opf.Manifest == nil {
		return 0, nil
	}

	coverFileName, err := e.coverFileNameFromOPF(opf, r)
	if err != nil {
		return 0, err
	}
	coverPaths := candidatePaths(coverFileName, opfBaseDir(r))

	// Count each distinct image file only once (manifest may reference the same file multiple times)
	seen := make(map[string]struct{})
	var count int
	for _, item := range opf.Manifest.Items {
		if !strings.HasPrefix(item.MediaType, "image/") {
			continue
		}
		resolved := resolveHref(item.Href, opfBaseDir(r))
		candidates := candidatePaths(resolved, "")
		zipPath := findZipEntryPath(r, candidates)
		if zipPath == "" {
			continue
		}
		if _, isCover := coverPaths[zipPath]; isCover {
			continue
		}
		if _, alreadyCounted := seen[zipPath]; alreadyCounted {
			continue
		}
		mp, err := imageMegapixels(r, zipPath)
		if err != nil {
			continue
		}
		if mp >= minMegapixels {
			seen[zipPath] = struct{}{}
			count++
		}
	}
	return count, nil
}

// candidatePaths returns possible zip paths for a file (used for both cover and image lookup).
// When opfBaseDir is non-empty, also adds opfBaseDir/fileOrPath.
func candidatePaths(fileOrPath, opfBaseDir string) map[string]struct{} {
	candidates := map[string]struct{}{}
	if fileOrPath != "" {
		candidates[fileOrPath] = struct{}{}
	}
	if opfBaseDir != "" && fileOrPath != "" {
		candidates[path.Clean(path.Join(opfBaseDir, fileOrPath))] = struct{}{}
	}
	if fileOrPath != "" && !strings.Contains(fileOrPath, "/") {
		candidates[path.Join("OEBPS", fileOrPath)] = struct{}{}
		candidates[path.Join("OPS", fileOrPath)] = struct{}{}
	}
	return candidates
}

func findZipEntryPath(r *zip.ReadCloser, candidates map[string]struct{}) string {
	for _, f := range r.File {
		if _, ok := candidates[f.Name]; ok {
			return f.Name
		}
	}
	return ""
}

func imageMegapixels(r *zip.ReadCloser, zipPath string) (float64, error) {
	rc, err := readZipFileReader(r, zipPath)
	if err != nil || rc == nil {
		return 0, err
	}
	cfg, _, err := image.DecodeConfig(rc)
	rc.Close()
	if err == nil {
		return float64(cfg.Width*cfg.Height) / 1e6, nil
	}
	return 0, err
}

func readZipFileReader(r *zip.ReadCloser, name string) (io.ReadCloser, error) {
	for _, f := range r.File {
		if f.Name != name {
			continue
		}
		return f.Open()
	}
	return nil, fmt.Errorf("epub: no zip entry %q", name)
}

func wordsFromZip(r *zip.ReadCloser) (int, error) {
	count := 0
	for _, f := range r.File {
		isContent, err := doublestar.PathMatch("O*PS/**/*.*htm*", f.Name)
		if err != nil {
			return 0, err
		}
		if !isContent {
			continue
		}

		if filepath.Base(f.Name) == "nav.xhtml" {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return 0, err
		}
		content, err := io.ReadAll(rc)
		if err != nil {
			return 0, err
		}

		p := bluemonday.StrictPolicy()
		text := p.Sanitize(string(content))
		words := strings.Fields(text)
		count += len(words)
		rc.Close()
	}
	return count, nil
}

func extractCover(r *zip.ReadCloser, coverFile, opfBaseDir string, coverMaxWidth int) ([]byte, error) {
	candidates := candidatePaths(coverFile, opfBaseDir)
	for _, f := range r.File {
		if _, ok := candidates[f.Name]; !ok {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		src, err := imaging.Decode(rc, imaging.Backends(imaging.GO_IMAGE))
		if err != nil {
			return nil, err
		}
		return resize(src, coverMaxWidth, err)
	}
	return nil, fmt.Errorf("no cover image found")
}

func selectCoverFileName(opf *epub.PackageDocument) string {
	if opf == nil || opf.Metadata == nil || opf.Manifest == nil {
		return ""
	}
	if coverHref := coverFromProperties(opf); coverHref != "" {
		return coverHref
	}
	if coverHref := coverFromMeta(opf); coverHref != "" {
		return coverHref
	}
	if coverHref := coverFromHeuristics(opf); coverHref != "" {
		return coverHref
	}
	return ""
}

func coverFromMeta(opf *epub.PackageDocument) string {
	for _, val := range opf.Metadata.Meta {
		if !strings.EqualFold(val.Name, "cover") || val.Content == "" {
			continue
		}
		for _, item := range opf.Manifest.Items {
			if item.ID == val.Content {
				return item.Href
			}
		}
		if strings.Contains(val.Content, ".") || strings.Contains(val.Content, "/") {
			return val.Content
		}
	}
	return ""
}

func coverFromProperties(opf *epub.PackageDocument) string {
	for _, item := range opf.Manifest.Items {
		if strings.Contains(strings.ToLower(item.Properties), "cover-image") {
			return item.Href
		}
	}
	return ""
}

func coverFromHeuristics(opf *epub.PackageDocument) string {
	for _, item := range opf.Manifest.Items {
		if !strings.HasPrefix(item.MediaType, "image/") {
			continue
		}
		id := strings.ToLower(item.ID)
		href := strings.ToLower(item.Href)
		if strings.Contains(id, "cover") || strings.Contains(href, "cover") {
			return item.Href
		}
	}
	return ""
}

func findOpfPath(r *zip.ReadCloser) string {
	for _, f := range r.File {
		if strings.HasSuffix(strings.ToLower(f.Name), ".opf") {
			return f.Name
		}
	}
	return ""
}

func resolveHref(href, baseDir string) string {
	if href == "" {
		return ""
	}
	href = strings.TrimPrefix(href, "/")
	if baseDir == "" {
		return path.Clean(href)
	}
	return path.Clean(path.Join(baseDir, href))
}

func isMarkupFile(href string) bool {
	ext := strings.ToLower(path.Ext(href))
	return ext == ".xhtml" || ext == ".html" || ext == ".htm" || ext == ".svg"
}

func findCoverImageInMarkup(r *zip.ReadCloser, markupPath string) (string, error) {
	content, err := readZipFile(r, markupPath)
	if err != nil {
		return "", err
	}
	if len(content) == 0 {
		return "", nil
	}
	baseDir := path.Dir(markupPath)
	if baseDir == "." {
		baseDir = ""
	}

	for _, pattern := range []string{
		`(?i)<img[^>]+src=["']([^"']+)["']`,
		`(?i)<image[^>]+xlink:href=["']([^"']+)["']`,
	} {
		re := regexp.MustCompile(pattern)
		if match := re.FindSubmatch(content); len(match) > 1 {
			src := string(match[1])
			if !strings.HasPrefix(strings.ToLower(src), "data:") {
				return resolveHref(src, baseDir), nil
			}
		}
	}
	return "", nil
}

// normalizeMarcRelator returns a short MARC relator code from OPF role values, which may be
// bare codes (e.g. "ill") or full vocabulary URIs (e.g. ".../relators/ill").
func normalizeMarcRelator(role string) string {
	r := strings.ToLower(strings.TrimSpace(role))
	if r == "" {
		return ""
	}
	if idx := strings.LastIndex(r, "/"); idx >= 0 {
		r = r[idx+1:]
	}
	r = strings.TrimPrefix(r, "marc:")
	switch r {
	case "illustrator":
		return "ill"
	case "artist":
		return "art"
	}
	return r
}

// classifyEpubPerson maps dc:creator / dc:contributor entries to authors or illustrators.
// Contributors with role ill (or art) are illustrators; empty role only counts as author on creators.
func classifyEpubPerson(p epub.Author, fromCreator bool, authors, illustrators *[]string) {
	switch normalizeMarcRelator(p.Role) {
	case "ill", "art":
		*illustrators = append(*illustrators, ParseAuthorList(p.FullName)...)
	case "aut":
		*authors = append(*authors, ParseAuthorList(p.FullName)...)
	case "":
		if fromCreator {
			*authors = append(*authors, ParseAuthorList(p.FullName)...)
		}
	default:
		// e.g. trl (translator), edt (editor)
	}
}

func readZipFile(r *zip.ReadCloser, name string) ([]byte, error) {
	for _, f := range r.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	return nil, nil
}
