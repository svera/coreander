package metadata

import (
	"archive/zip"
	"bytes"
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
			// Some epub files mistakenly put all authors in a single field instead of using a field for each one.
			// We want to identify those cases looking for specific separators and then indexing each author properly.
			names := strings.SplitSeq(creator.FullName, "&")
			for name := range names {
				if name = strings.TrimSpace(name); name != "" {
					authors = append(authors, name)
				}
			}
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
		names := strings.Split(subject, ",")
		for _, name := range names {
			if name = strings.TrimSpace(name); name != "" {
				subjects = append(subjects, name)
			}
		}
	}

	description := ""
	if len(meta.Description) > 0 {
		strict := bluemonday.StrictPolicy()
		noHTMLDescription := strict.Sanitize(meta.Description[0])
		if noHTMLDescription == meta.Description[0] {
			paragraphs := strings.Split(meta.Description[0], "\n")
			description = "<p>" + strings.Join(paragraphs, "</p><p>") + "</p>"
		} else {
			p := bluemonday.UGCPolicy()
			description = p.Sanitize(meta.Description[0])
		}
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

	bk = Metadata{
		Title:       title,
		Authors:     authors,
		Description: template.HTML(description),
		Language:    lang,
		Publication: publication,
		Series:      meta.Series,
		SeriesIndex: seriesIndex,
		Format:      "EPUB",
		Subjects:    subjects,
	}
	w, err := words(filename)
	if err != nil {
		log.Printf("Cannot count words in %s: $%s\n", filename, err)
	}
	bk.Words = float64(w)
	return bk, nil
}

// Cover parses the document looking for a cover image and returns it
func (e EpubReader) Cover(documentFullPath string, coverMaxWidth int) ([]byte, error) {
	var cover []byte

	coverFileName := ""

	opf, err := e.GetPackageFromFile(documentFullPath)
	if err != nil {
		return nil, err
	}

	r, err := zip.OpenReader(documentFullPath)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	opfPath := findOpfPath(r)
	opfBaseDir := ""
	if opfPath != "" {
		opfBaseDir = path.Dir(opfPath)
		if opfBaseDir == "." {
			opfBaseDir = ""
		}
	}

	coverFileName = selectCoverFileName(opf)
	if coverFileName == "" {
		return nil, fmt.Errorf("no cover image set in %s", documentFullPath)
	}

	coverFileName = resolveHref(coverFileName, opfBaseDir)
	if isMarkupFile(coverFileName) {
		if resolvedCover, err := findCoverImageInMarkup(r, coverFileName); err == nil && resolvedCover != "" {
			coverFileName = resolvedCover
		}
	}

	cover, err = extractCover(r, coverFileName, opfBaseDir, coverMaxWidth)
	if err != nil {
		return nil, err
	}
	return cover, nil
}

// Illustrations returns the number of images in the EPUB that have size >= minMegapixels (excluding the cover).
func (e EpubReader) Illustrations(documentFullPath string, minMegapixels float64) (int, error) {
	opf, err := e.GetPackageFromFile(documentFullPath)
	if err != nil {
		return 0, err
	}
	r, err := zip.OpenReader(documentFullPath)
	if err != nil {
		return 0, err
	}
	defer r.Close()

	opfPath := findOpfPath(r)
	opfBaseDir := ""
	if opfPath != "" {
		opfBaseDir = path.Dir(opfPath)
		if opfBaseDir == "." {
			opfBaseDir = ""
		}
	}

	coverFileName := selectCoverFileName(opf)
	coverFileName = resolveHref(coverFileName, opfBaseDir)
	if isMarkupFile(coverFileName) {
		if resolvedCover, err := findCoverImageInMarkup(r, coverFileName); err == nil && resolvedCover != "" {
			coverFileName = resolvedCover
		}
	}
	coverPaths := coverCandidatePaths(coverFileName, opfBaseDir)

	if opf.Manifest == nil {
		return 0, nil
	}
	zipByName := make(map[string]*zip.File, len(r.File))
	for _, f := range r.File {
		zipByName[f.Name] = f
	}
	// Count each distinct image file only once (manifest may reference the same file multiple times)
	seen := make(map[string]struct{})
	var count int
	for _, item := range opf.Manifest.Items {
		if !strings.HasPrefix(item.MediaType, "image/") {
			continue
		}
		resolved := resolveHref(item.Href, opfBaseDir)
		candidates := imageCandidatePaths(resolved)
		zipPath := findZipEntryPathFromMap(zipByName, candidates)
		if zipPath == "" {
			continue
		}
		if _, isCover := coverPaths[zipPath]; isCover {
			continue
		}
		if _, alreadyCounted := seen[zipPath]; alreadyCounted {
			continue
		}
		mp, err := imageMegapixelsFromZip(zipByName, zipPath)
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

func imageCandidatePaths(resolved string) map[string]struct{} {
	candidates := map[string]struct{}{}
	if resolved != "" {
		candidates[resolved] = struct{}{}
	}
	base := path.Base(resolved)
	if base != "" && base != "." && !strings.Contains(resolved, "/") {
		candidates[path.Join("OEBPS", base)] = struct{}{}
		candidates[path.Join("OPS", base)] = struct{}{}
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

func findZipEntryPathFromMap(zipByName map[string]*zip.File, candidates map[string]struct{}) string {
	for name := range candidates {
		if _, ok := zipByName[name]; ok {
			return name
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
	rc2, _ := readZipFileReader(r, zipPath)
	if rc2 != nil {
		defer rc2.Close()
		return imageMegapixelsFromReader(rc2)
	}
	return 0, err
}

// imageMegapixelsFromZip gets dimensions from the zip entry using a pre-built map (avoids repeated zip scans).
// Tries image.DecodeConfig first (header-only for JPEG/PNG/GIF); only reads and decodes the full file when needed.
func imageMegapixelsFromZip(zipByName map[string]*zip.File, zipPath string) (float64, error) {
	f, ok := zipByName[zipPath]
	if !ok {
		return 0, fmt.Errorf("epub: no zip entry %q", zipPath)
	}
	rc, err := f.Open()
	if err != nil {
		return 0, err
	}
	cfg, _, err := image.DecodeConfig(rc)
	rc.Close()
	if err == nil {
		return float64(cfg.Width*cfg.Height) / 1e6, nil
	}
	// Fallback: full read and decode for WebP, malformed headers, etc.
	rc2, err := f.Open()
	if err != nil {
		return 0, err
	}
	defer rc2.Close()
	return imageMegapixelsFromReader(rc2)
}

// imageMegapixelsFromReader returns megapixels by full decode (used when DecodeConfig is not enough).
func imageMegapixelsFromReader(rc io.Reader) (float64, error) {
	data, err := io.ReadAll(rc)
	if err != nil {
		return 0, err
	}
	img, err := imaging.Decode(bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	b := img.Bounds()
	return float64(b.Dx()*b.Dy()) / 1e6, nil
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
	candidates := coverCandidatePaths(coverFile, opfBaseDir)
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

func coverCandidatePaths(coverFile, opfBaseDir string) map[string]struct{} {
	candidates := map[string]struct{}{}
	if coverFile != "" {
		candidates[coverFile] = struct{}{}
	}
	if opfBaseDir != "" && coverFile != "" {
		candidates[path.Clean(path.Join(opfBaseDir, coverFile))] = struct{}{}
	}
	if coverFile != "" && !strings.Contains(coverFile, "/") {
		candidates[path.Join("OEBPS", coverFile)] = struct{}{}
		candidates[path.Join("OPS", coverFile)] = struct{}{}
	}
	return candidates
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
