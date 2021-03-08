package metadata

import (
	"archive/zip"
	"html/template"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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
	title := ""
	if len(opf.Metadata.Title) > 0 {
		title = opf.Metadata.Title[0]
	}
	author := ""
	if len(opf.Metadata.Creator) > 0 {
		for _, creator := range opf.Metadata.Creator {
			if creator.Role == "aut" {
				author = creator.FullName
			}
		}
	}
	description := ""
	if len(opf.Metadata.Description) > 0 {
		description = opf.Metadata.Description[0]
	}
	language := ""
	if len(opf.Metadata.Language) > 0 {
		language = opf.Metadata.Language[0]
	}
	year := ""
	if len(opf.Metadata.Date) > 0 {
		for _, date := range opf.Metadata.Date {
			if date.Event == "publication" {
				t, err := time.Parse("2006-01-02", date.Stamp)
				if err == nil {
					year = t.Format("2006")
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
		Author:      author,
		Description: template.HTML(description),
		Language:    language,
		Year:        year,
		Cover:       cover,
		Series:      series,
		SeriesIndex: seriesIndex,
	}
	w, err := words(file)
	if err != nil {
		log.Println(err)
	}
	bk.Words = float64(w)
	return bk, nil
}

func words(bookFullPath string) (int, error) {
	r, err := zip.OpenReader(bookFullPath)
	if err != nil {
		return 0, err
	}
	defer r.Close()
	count := 0
	for _, f := range r.File {
		isContent, err := filepath.Match("OEBPS/Text/*.*html", f.Name)
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
