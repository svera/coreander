package metadata

import (
	"archive/zip"
	"fmt"
	"html/template"
	"io"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/disintegration/imaging"
	"github.com/gofiber/fiber/v2"
	"github.com/microcosm-cc/bluemonday"
	"github.com/pirmd/epub"
)

type EpubReader struct{}

func (e EpubReader) Metadata(file string) (Metadata, error) {
	bk := Metadata{}
	opf, err := epub.GetPackageFromFile(file)
	if err != nil {
		return bk, err
	}
	title := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	if len(opf.Metadata.Title) > 0 && len(opf.Metadata.Title[0].Value) > 0 {
		title = opf.Metadata.Title[0].Value
	}
	var authors []string
	if len(opf.Metadata.Creator) > 0 {
		for _, creator := range opf.Metadata.Creator {
			if creator.Role == "aut" || creator.Role == "" {
				// Some epub files mistakenly put all authors in a single field instead of using a field for each one.
				// We want to identify those cases looking for specific separators and then indexing each author properly.
				names := strings.Split(creator.Value, "&")
				for i := range names {
					names[i] = strings.TrimSpace(names[i])
				}
				authors = append(authors, names...)
			}
		}
	}

	if len(authors) == 0 {
		authors = []string{""}
	}

	var subjects []string
	if len(opf.Metadata.Subject) > 0 {
		for _, subject := range opf.Metadata.Subject {
			subject.Value = strings.TrimSpace(subject.Value)
			if subject.Value == "" {
				continue
			}
			// Some epub files mistakenly put all subjects in a single field instead of using a field for each one.
			// We want to identify those cases looking for specific separators and then indexing each subject properly.
			names := strings.Split(subject.Value, ",")
			for i := range names {
				names[i] = strings.TrimSpace(names[i])
			}
			subjects = append(subjects, names...)
		}
	}

	description := ""
	if len(opf.Metadata.Description) > 0 {
		strict := bluemonday.StrictPolicy()
		noHTMLDescription := strict.Sanitize(opf.Metadata.Description[0].Value)
		if noHTMLDescription == opf.Metadata.Description[0].Value {
			paragraphs := strings.Split(opf.Metadata.Description[0].Value, "\n")
			description = "<p>" + strings.Join(paragraphs, "</p><p>") + "</p>"
		} else {
			p := bluemonday.UGCPolicy()
			description = p.Sanitize(opf.Metadata.Description[0].Value)
		}
	}

	lang := ""
	if len(opf.Metadata.Language) > 0 {
		lang = opf.Metadata.Language[0].Value
	}

	year := ""
	if len(opf.Metadata.Date) > 0 {
		for _, date := range opf.Metadata.Date {
			if date.Event == "publication" || date.Event == "" {
				t, err := time.Parse("2006-01-02", date.Value)
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
			for _, item := range opf.Manifest.Items {
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
		Language:    lang,
		Year:        year,
		Cover:       cover,
		Series:      series,
		SeriesIndex: seriesIndex,
		Type:        "EPUB",
		Subjects:    subjects,
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

func extractCover(r *zip.ReadCloser, coverFile string, coverMaxWidth int) ([]byte, error) {
	for _, f := range r.File {
		if f.Name != fmt.Sprintf("OEBPS/%s", coverFile) && f.Name != fmt.Sprintf("OPS/%s", coverFile) && f.Name != coverFile {
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
