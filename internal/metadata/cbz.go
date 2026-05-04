package metadata

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"html/template"
	"io"
	"log"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rickb777/date/v2"
	"github.com/svera/coreander/v4/internal/precisiondate"
)

// ComicInfo represents the ComicInfo.xml schema (ComicRack/Anansi) used in CBZ archives.
type ComicInfo struct {
	XMLName xml.Name `xml:"ComicInfo"`

	Title   string `xml:"Title"`
	Series  string `xml:"Series"`
	Number  string `xml:"Number"`
	Count   int    `xml:"Count"`
	Volume  int    `xml:"Volume"`
	Summary string `xml:"Summary"`
	Notes   string `xml:"Notes"`

	Year  int `xml:"Year"`
	Month int `xml:"Month"`
	Day   int `xml:"Day"`

	Writer      string `xml:"Writer"`
	Penciller   string `xml:"Penciller"`
	Inker       string `xml:"Inker"`
	Colorist    string `xml:"Colorist"`
	Letterer    string `xml:"Letterer"`
	CoverArtist string `xml:"CoverArtist"`
	// Illustrator is used by some tools alongside or instead of Penciller/Inker.
	Illustrator string `xml:"Illustrator"`
	Editor      string `xml:"Editor"`
	Publisher   string `xml:"Publisher"`
	Imprint     string `xml:"Imprint"`
	Genre       string `xml:"Genre"`
	Web         string `xml:"Web"`
	PageCount   int    `xml:"PageCount"`
	LanguageISO string `xml:"LanguageISO"`
	Format      string `xml:"Format"`
	Characters  string `xml:"Characters"`
	Teams       string `xml:"Teams"`
	Locations   string `xml:"Locations"`
	StoryArc    string `xml:"StoryArc"`
	SeriesGroup string `xml:"SeriesGroup"`
	ScanInfo    string `xml:"ScanInformation"`
	AgeRating   string `xml:"AgeRating"`
	Review      string `xml:"Review"`

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

	title := DefaultTitleFromFilename(file)
	if info != nil && strings.TrimSpace(info.Title) != "" {
		title = strings.TrimSpace(info.Title)
	}

	authors := []string{""}
	var illustrators []string
	if info != nil {
		authors = AuthorsOrEmptySlot(collectComicAuthors(info))
		illustrators = collectComicIllustrators(info)
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
		seriesIndex = ParseSeriesIndex(info.Number)
	}

	pages := float64(len(SortedImageEntriesFromZip(r)))
	if info != nil && info.PageCount > 0 {
		pages = float64(info.PageCount)
	}

	var subjects []string
	if info != nil && info.Genre != "" {
		subjects = ParseSubjectList(info.Genre)
	}

	formatLabel := "CBZ"
	if info != nil && info.Format != "" {
		formatLabel = strings.TrimSpace(info.Format)
	}

	illustrations, err := c.illustrations(file, 0.25)
	if err != nil {
		log.Printf("Cannot count illustrations in %s: %v\n", file, err)
	}

	bk = Metadata{
		Title:         title,
		Authors:       authors,
		Illustrators:  illustrators,
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
	coverName, names := cbzCoverImageAndNames(r, info)
	if len(names) == 0 {
		return nil, fmt.Errorf("cbz: no image found")
	}

	return DecodeResizeZipImageEntry(r, coverName, coverMaxWidth)
}

// illustrations returns the number of images in the CBZ with size >= minMegapixels (excluding the cover).
func (c CbzReader) illustrations(documentFullPath string, minMegapixels float64) (int, error) {
	r, err := zip.OpenReader(documentFullPath)
	if err != nil {
		return 0, err
	}
	defer r.Close()

	info, _ := readComicInfoFromZip(r)
	coverName, names := cbzCoverImageAndNames(r, info)
	if len(names) == 0 {
		return 0, nil
	}

	var count int
	for _, name := range names {
		if name == coverName {
			continue
		}
		mp, err := ImageMegapixelsFromZip(r, name)
		if err != nil {
			continue
		}
		if mp >= minMegapixels {
			count++
		}
	}
	return count, nil
}

// cbzCoverImageAndNames returns sorted image paths in the archive and the path used as cover
// (ComicInfo FrontCover index when valid, otherwise the first image).
func cbzCoverImageAndNames(r *zip.ReadCloser, info *ComicInfo) (coverName string, names []string) {
	names = SortedImageEntriesFromZip(r)
	if len(names) == 0 {
		return "", nil
	}
	coverIndex := 1
	if info != nil && info.Pages != nil {
		for _, p := range info.Pages.Page {
			if strings.EqualFold(strings.TrimSpace(p.Type), "FrontCover") {
				coverIndex = p.Image
				break
			}
		}
	}
	if coverIndex < 1 || coverIndex > len(names) {
		coverIndex = 1
	}
	return names[coverIndex-1], names
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

	rc, err := OpenZipEntry(r, entryName)
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

// collectComicAuthors returns writing and editorial credits (ComicInfo Writer, Editor).
func collectComicAuthors(info *ComicInfo) []string {
	return uniqueComicNames(info.Writer, info.Editor)
}

// collectComicIllustrators returns art credits from ComicInfo: penciller, inker,
// colorist, letterer, cover artist, and optional Illustrator element.
func collectComicIllustrators(info *ComicInfo) []string {
	return uniqueComicNames(
		info.Penciller,
		info.Inker,
		info.Colorist,
		info.Letterer,
		info.CoverArtist,
		info.Illustrator,
	)
}

func uniqueComicNames(fields ...string) []string {
	var combined []string
	for _, s := range fields {
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
