package metadata

import (
	"archive/zip"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/disintegration/imaging"
	"github.com/gofiber/fiber/v2"
	"github.com/microcosm-cc/bluemonday"
	"github.com/svera/epub"
)

type EpubReader struct{}

func (e EpubReader) Metadata(file string) (Metadata, error) {
	bk := Metadata{}
	r, err := os.Open(file)
	if err != nil {
		return bk, err
	}
	defer r.Close()
	opf, err := epub.GetOPFData(r)
	if err != nil {
		return bk, err
	}
	title := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	if len(opf.Metadata.Title) > 0 && len(opf.Metadata.Title[0]) > 0 {
		title = opf.Metadata.Title[0]
	}
	var authors []string
	if len(opf.Metadata.Creator) > 0 {
		for _, creator := range opf.Metadata.Creator {
			if creator.Role == "aut" || creator.Role == "" {
				names := strings.Split(creator.FullName, "&")
				//names = strings.Split(strings.Join(names, " "), ",")
				for i := range names {
					names[i] = strings.TrimSpace(names[i])
				}
				authors = append(authors, names...)
			}
		}
	}

	description := ""
	if len(opf.Metadata.Description) > 0 {
		p := bluemonday.UGCPolicy()
		description = p.Sanitize(opf.Metadata.Description[0])
	}
	language := ""
	if len(opf.Metadata.Language) > 0 {
		language = opf.Metadata.Language[0]
	}
	year := ""
	if len(opf.Metadata.Date) > 0 {
		for _, date := range opf.Metadata.Date {
			if date.Event == "publication" || date.Event == "" {
				t, err := time.Parse("2006-01-02", date.Stamp)
				if err == nil {
					year = strings.TrimLeft(t.Format("2006"), "0")
					break
				}
			}
		}
	}
	cover := ""
	series := ""
	var seriesIndex float64 = 0
	for _, val := range opf.Metadata.Meta {
		if val.Name == "cover" {
			id := val.Content
			for _, item := range opf.Manifest.Item {
				if item.ID == id {
					cover = item.Href
					break
				}
			}
		}
		if val.Name == "calibre:series" {
			series = val.Content
		}
		if val.Name == "calibre:series_index" {
			seriesIndex, _ = strconv.ParseFloat(val.Content, 64)
		}
	}
	bk = Metadata{
		Title:       title,
		Authors:     authors,
		Description: template.HTML(description),
		Language:    language,
		Year:        year,
		Cover:       cover,
		Series:      series,
		SeriesIndex: seriesIndex,
		Type:        "EPUB",
	}
	w, err := words(file)
	if err != nil {
		log.Println(err)
	}
	bk.Words = float64(w)
	return bk, nil
}

// Cover parses the document looking for a cover image and returns it
func (e EpubReader) Cover(documentFullPath string, coverMaxWidth int) ([]byte, error) {
	var cover []byte

	reader := EpubReader{}
	meta, err := reader.Metadata(documentFullPath)
	if err != nil {
		return nil, err
	}
	if meta.Cover == "" {
		return nil, fmt.Errorf("no cover image set in %s", documentFullPath)
	}

	r, err := zip.OpenReader(documentFullPath)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	cover, err = extractCover(r, meta.Cover, coverMaxWidth)
	if err != nil {
		return nil, err
	}
	return cover, nil
}

func words(bookFullPath string) (int, error) {
	r, err := zip.OpenReader(bookFullPath)
	if err != nil {
		return 0, err
	}
	defer r.Close()
	count := 0
	for _, f := range r.File {
		isContent, err := doublestar.PathMatch("OEBPS/**/*.*html", f.Name)
		if err != nil {
			return 0, err
		}
		if !isContent {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return 0, err
		}
		content, err := ioutil.ReadAll(rc)
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

func extractCover(r *zip.ReadCloser, coverFile string, coverMaxWidth int) ([]byte, error) {
	for _, f := range r.File {
		if f.Name != fmt.Sprintf("OEBPS/%s", coverFile) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		src, err := imaging.Decode(rc)
		if err != nil {
			return nil, fiber.ErrInternalServerError
		}
		return resize(src, coverMaxWidth, err)
	}
	return nil, fmt.Errorf("no cover image found")
}
