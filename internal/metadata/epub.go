package metadata

import (
	"archive/zip"
	"html/template"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
	"github.com/pirmd/epub"
)

type EpubReader struct{}

func (e EpubReader) Metadata(file string) (Metadata, error) {
	bk := Metadata{}
	metadata, err := epub.GetMetadataFromFile(file)
	if err != nil {
		return bk, err
	}
	title := ""
	if len(metadata.Title) > 0 {
		title = metadata.Title[0]
	}
	author := ""
	if len(metadata.Creator) > 0 {
		author = metadata.Creator[0].FullName
	}
	description := ""
	if len(metadata.Description) > 0 {
		description = metadata.Description[0]
	}
	language := ""
	if len(metadata.Language) > 0 {
		language = metadata.Language[0]
	}
	year := ""
	if len(metadata.Date) > 0 {
		t, err := time.Parse("2006-01-02", metadata.Date[0].Stamp)
		if err == nil {
			year = t.Format("2006")
		}
	}
	cover := ""
	for _, val := range metadata.Meta {
		if val.Name == "cover" {
			cover = val.Content
		}
	}
	bk = Metadata{
		Title:       title,
		Author:      author,
		Description: template.HTML(description),
		Language:    language,
		Year:        year,
		Cover:       cover,
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
