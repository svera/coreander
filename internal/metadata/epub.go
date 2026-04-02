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
	bk := Metadata{}
	meta, err := e.GetMetadataFromFile(filename)
	if err != nil {
		return bk, err
	}
	title := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	if len(meta.Title) > 0 && len(meta.Title[0]) > 0 {
		title = meta.Title[0]
	}
	var authors []string
	for _, creator := range meta.Creator {
		if creator.Role == "aut" || creator.Role == "" {
			authors = append(authors, ParseAuthorList(creator.FullName)...)
		}
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
		// Some epub files mistakenly put all subjects in a single field instead of using a field for each one.
		// We want to identify those cases looking for specific separators and then indexing each subject properly.
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
	for _, currentDate := range meta.Date {
		if currentDate.Event == "publication" || currentDate.Event == "" {
			if publication.Date, err = date.ParseISO(currentDate.Stamp); err != nil {
				publication.Precision = precisiondate.PrecisionYear
				publication.Date, _ = date.Parse("2006", currentDate.Stamp)
			}
			break
		}
	}

	var seriesIndex float64 = 0

	seriesIndex, _ = strconv.ParseFloat(meta.SeriesIndex, 64)

	illustrations, err := e.illustrations(filename, 0.25)
	if err != nil {
		log.Printf("Cannot count illustrations in %s: $%s\n", filename, err)
	}

	w, err := words(filename)
	if err != nil {
		log.Printf("Cannot count words in %s: $%s\n", filename, err)
	}

	bk = Metadata{
		Title:         title,
		Authors:       authors,
		Description:   template.HTML(description),
		Language:      lang,
		Publication:   publication,
		Series:        meta.Series,
		SeriesIndex:   seriesIndex,
		Format:        "EPUB",
		Subjects:      subjects,
		Illustrations: illustrations,
		Words:         float64(w),
	}
	return bk, nil
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

func (e EpubReader) coverFileName(documentFullPath string, r *zip.ReadCloser) (string, error) {
	opf, err := e.GetPackageFromFile(documentFullPath)
	if err != nil {
		return "", err
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

	r, err := zip.OpenReader(documentFullPath)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	coverFileName, err := e.coverFileName(documentFullPath, r)
	if err != nil {
		return nil, err
	}

	cover, err = extractCover(r, coverFileName, opfBaseDir(r), coverMaxWidth)
	if err != nil {
		return nil, err
	}
	return cover, nil
}

// illustrations returns the number of images in the EPUB that have size >= minMegapixels (excluding the cover).
func (e EpubReader) illustrations(documentFullPath string, minMegapixels float64) (int, error) {
	opf, err := e.GetPackageFromFile(documentFullPath)
	if err != nil {
		return 0, err
	}
	r, err := zip.OpenReader(documentFullPath)
	if err != nil {
		return 0, err
	}
	defer r.Close()

	coverFileName, err := e.coverFileName(documentFullPath, r)
	if err != nil {
		return 0, err
	}
	coverPaths := candidatePaths(coverFileName, opfBaseDir(r))

	if opf.Manifest == nil {
		return 0, nil
	}
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

func words(documentFullPath string) (int, error) {
	r, err := zip.OpenReader(documentFullPath)
	if err != nil {
		return 0, err
	}
	defer r.Close()
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
		src, err := imaging.Decode(rc)
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
