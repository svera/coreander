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
	meta, err := epub.GetMetadataFromFile(file)
	if err != nil {
		return bk, err
	}
	title := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	if len(meta.Title) > 0 && len(meta.Title[0]) > 0 {
		title = meta.Title[0]
	}
	var authors []string
	if len(meta.Creator) > 0 {
		for _, creator := range meta.Creator {
			if creator.Role == "aut" || creator.Role == "" {
				// Some epub files mistakenly put all authors in a single field instead of using a field for each one.
				// We want to identify those cases looking for specific separators and then indexing each author properly.
				names := strings.Split(creator.FullName, "&")
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
	if len(meta.Subject) > 0 {
		for _, subject := range meta.Subject {
			subject = strings.TrimSpace(subject)
			if subject == "" {
				continue
			}
			// Some epub files mistakenly put all subjects in a single field instead of using a field for each one.
			// We want to identify those cases looking for specific separators and then indexing each subject properly.
			names := strings.Split(subject, ",")
			for i := range names {
				names[i] = strings.TrimSpace(names[i])
			}
			subjects = append(subjects, names...)
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

	year := ""
	if len(meta.Date) > 0 {
		for _, date := range meta.Date {
			if date.Event == "publication" || date.Event == "" {
				t, err := time.Parse("2006-01-02", date.Stamp)
				if err == nil {
					year = strings.TrimLeft(t.Format("2006"), "0")
					break
				}
			}
		}
	}
	var seriesIndex float64 = 0

	seriesIndex, _ = strconv.ParseFloat(meta.SeriesIndex, 64)

	bk = Metadata{
		Title:       title,
		Authors:     authors,
		Description: template.HTML(description),
		Language:    lang,
		Year:        year,
		Series:      meta.Series,
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

	coverFileName := ""

	opf, err := epub.GetPackageFromFile(documentFullPath)
	if err != nil {
		return nil, err
	}

	for _, val := range opf.Metadata.Meta {
		if val.Name != "cover" {
			continue
		}
		for _, item := range opf.Manifest.Items {
			if item.ID == val.Content {
				coverFileName = item.Href
				break
			}
		}
	}

	if coverFileName == "" {
		return nil, fmt.Errorf("no cover image set in %s", documentFullPath)
	}

	r, err := zip.OpenReader(documentFullPath)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	cover, err = extractCover(r, coverFileName, coverMaxWidth)
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
