package metadata

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"html/template"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/kovidgoyal/imaging"
	"github.com/rickb777/date/v2"
	"github.com/svera/coreander/v4/internal/precisiondate"
)

// ComicInfo represents the ComicInfo.xml schema (ComicRack/Anansi) used in CBZ archives.
type ComicInfo struct {
	XMLName xml.Name `xml:"ComicInfo"`

	Title    string `xml:"Title"`
	Series   string `xml:"Series"`
	Number   string `xml:"Number"`
	Count    int    `xml:"Count"`
	Volume   int    `xml:"Volume"`
	Summary  string `xml:"Summary"`
	Notes    string `xml:"Notes"`

	Year  int `xml:"Year"`
	Month int `xml:"Month"`
	Day   int `xml:"Day"`

	Writer       string `xml:"Writer"`
	Penciller    string `xml:"Penciller"`
	Inker        string `xml:"Inker"`
	Colorist     string `xml:"Colorist"`
	Letterer     string `xml:"Letterer"`
	CoverArtist  string `xml:"CoverArtist"`
	Editor       string `xml:"Editor"`
	Publisher    string `xml:"Publisher"`
	Imprint      string `xml:"Imprint"`
	Genre        string `xml:"Genre"`
	Web          string `xml:"Web"`
	PageCount    int    `xml:"PageCount"`
	LanguageISO  string `xml:"LanguageISO"`
	Format       string `xml:"Format"`
	Characters   string `xml:"Characters"`
	Teams        string `xml:"Teams"`
	Locations    string `xml:"Locations"`
	StoryArc     string `xml:"StoryArc"`
	SeriesGroup  string `xml:"SeriesGroup"`
	ScanInfo     string `xml:"ScanInformation"`
	AgeRating    string `xml:"AgeRating"`
	Review       string `xml:"Review"`

	Pages *ComicPages `xml:"Pages"`
}

// ComicPages holds the optional list of page descriptors (cover index, etc.).
type ComicPages struct {
	Page []ComicPageInfo `xml:"Page"`
}

// ComicPageInfo describes a single page (e.g. FrontCover at a given image index).
type ComicPageInfo struct {
	Image int    `xml:"Image,attr"`
	Type  string `xml:"Type,attr"`
}

type CbzReader struct{}

// comicInfoFilenames are possible names for the metadata file (case-insensitive match).
var comicInfoFilenames = []string{"ComicInfo.xml", "comicinfo.xml", "COMICINFO.XML"}

func (c CbzReader) Metadata(file string) (Metadata, error) {
	bk := Metadata{}

	r, err := zip.OpenReader(file)
	if err != nil {
		return bk, err
	}
	defer r.Close()

	info, _ := readComicInfoFromZip(r)

	title := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	if info != nil && strings.TrimSpace(info.Title) != "" {
		title = strings.TrimSpace(info.Title)
	}

	authors := []string{""}
	if info != nil {
		authors = collectComicAuthors(info)
		if len(authors) == 0 {
			authors = []string{""}
		}
	}

	description := ""
	if info != nil {
		if info.Summary != "" {
			description = SanitizeDescription(info.Summary)
		} else if info.Notes != "" {
			description = SanitizeDescription(info.Notes)
		}
	}

	lang := ""
	if info != nil {
		lang = strings.TrimSpace(info.LanguageISO)
	}

	publication := precisiondate.PrecisionDate{Precision: precisiondate.PrecisionDay}
	if info != nil && info.Year > 0 {
		if info.Month > 0 && info.Month <= 12 && info.Day > 0 && info.Day <= 31 {
			publication.Date, _ = date.Parse("2006-01-02", fmt.Sprintf("%04d-%02d-%02d", info.Year, info.Month, info.Day))
		} else {
			publication.Precision = precisiondate.PrecisionYear
			publication.Date, _ = date.Parse("2006", strconv.Itoa(info.Year))
		}
	}

	seriesIndex := 0.0
	series := ""
	if info != nil {
		series = strings.TrimSpace(info.Series)
		if info.Number != "" {
			if n, err := strconv.ParseFloat(strings.TrimSpace(info.Number), 64); err == nil {
				seriesIndex = n
			}
		}
	}

	pages := float64(countImageEntries(r))
	if info != nil && info.PageCount > 0 {
		pages = float64(info.PageCount)
	}

	var subjects []string
	if info != nil && info.Genre != "" {
		for _, s := range strings.FieldsFunc(info.Genre, func(r rune) bool { return r == ',' || r == ';' }) {
			if s = strings.TrimSpace(s); s != "" {
				subjects = append(subjects, s)
			}
		}
	}

	formatLabel := "CBZ"
	if info != nil && info.Format != "" {
		formatLabel = strings.TrimSpace(info.Format)
	}

	illustrations, err := c.illustrations(file, 0.25)
	if err != nil {
		log.Printf("Cannot count illustrations in %s: %s\n", file, err)
	}

	bk = Metadata{
		Title:         title,
		Authors:       authors,
		Description:   template.HTML(description),
		Language:      lang,
		Publication:   publication,
		Series:        series,
		SeriesIndex:   seriesIndex,
		Pages:         pages,
		Format:        formatLabel,
		Subjects:      subjects,
		Illustrations: illustrations,
	}
	return bk, nil
}

func (c CbzReader) Cover(documentFullPath string, coverMaxWidth int) ([]byte, error) {
	r, err := zip.OpenReader(documentFullPath)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	info, _ := readComicInfoFromZip(r)
	coverIndex := 0
	if info != nil && info.Pages != nil {
		for _, p := range info.Pages.Page {
			if strings.EqualFold(strings.TrimSpace(p.Type), "FrontCover") {
				coverIndex = p.Image
				break
			}
		}
	}

	names := sortedImageEntries(r)
	if len(names) == 0 {
		return nil, fmt.Errorf("cbz: no image found")
	}
	if coverIndex < 1 || coverIndex > len(names) {
		coverIndex = 1
	}
	coverName := names[coverIndex-1]

	rc, err := openZipEntry(r, coverName)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	src, err := imaging.Decode(rc)
	if err != nil {
		return nil, err
	}
	return resize(src, coverMaxWidth, nil)
}

// illustrations returns the number of images in the CBZ with size >= minMegapixels (excluding the cover).
func (c CbzReader) illustrations(documentFullPath string, minMegapixels float64) (int, error) {
	r, err := zip.OpenReader(documentFullPath)
	if err != nil {
		return 0, err
	}
	defer r.Close()

	info, _ := readComicInfoFromZip(r)
	coverIndex := 0
	if info != nil && info.Pages != nil {
		for _, p := range info.Pages.Page {
			if strings.EqualFold(strings.TrimSpace(p.Type), "FrontCover") {
				coverIndex = p.Image
				break
			}
		}
	}

	names := sortedImageEntries(r)
	if len(names) == 0 {
		return 0, nil
	}
	if coverIndex < 1 || coverIndex > len(names) {
		coverIndex = 1
	}
	coverName := names[coverIndex-1]

	var count int
	for _, name := range names {
		if name == coverName {
			continue
		}
		mp, err := cbzImageMegapixels(r, name)
		if err != nil {
			continue
		}
		if mp >= minMegapixels {
			count++
		}
	}
	return count, nil
}

func cbzImageMegapixels(r *zip.ReadCloser, name string) (float64, error) {
	rc, err := openZipEntry(r, name)
	if err != nil || rc == nil {
		return 0, err
	}
	defer rc.Close()
	cfg, _, err := image.DecodeConfig(rc)
	if err != nil {
		return 0, err
	}
	return float64(cfg.Width*cfg.Height) / 1e6, nil
}

func readComicInfoFromZip(r *zip.ReadCloser) (*ComicInfo, error) {
	var entryName string
	for _, f := range r.File {
		base := filepath.Base(f.Name)
		for _, want := range comicInfoFilenames {
			if base == want {
				entryName = f.Name
				break
			}
		}
		if entryName != "" {
			break
		}
	}
	if entryName == "" {
		return nil, nil
	}

	rc, err := openZipEntry(r, entryName)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	var info ComicInfo
	if err := xml.Unmarshal(data, &info); err != nil {
		return nil, nil
	}
	return &info, nil
}

func collectComicAuthors(info *ComicInfo) []string {
	var combined []string
	for _, s := range []string{
		info.Writer, info.Penciller, info.Inker, info.CoverArtist,
		info.Colorist, info.Letterer, info.Editor,
	} {
		combined = append(combined, ParseAuthorList(s)...)
	}
	seen := make(map[string]struct{})
	var out []string
	for _, name := range combined {
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

var imageExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
	".webp": true, ".bmp": true, ".tiff": true, ".tif": true,
}

func sortedImageEntries(r *zip.ReadCloser) []string {
	var names []string
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(f.Name))
		if imageExtensions[ext] {
			names = append(names, f.Name)
		}
	}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	return names
}

func countImageEntries(r *zip.ReadCloser) int {
	return len(sortedImageEntries(r))
}

func openZipEntry(r *zip.ReadCloser, name string) (io.ReadCloser, error) {
	for _, f := range r.File {
		if f.Name != name {
			continue
		}
		return f.Open()
	}
	return nil, fmt.Errorf("cbz: entry %q not found", name)
}
