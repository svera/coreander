package metadata

import (
	"fmt"
	"html/template"
	"path/filepath"
	"strings"
	"time"

	"github.com/flotzilla/pdf_parser"
	"github.com/microcosm-cc/bluemonday"
)

type PdfReader struct{}

func (p PdfReader) Metadata(file string) (Metadata, error) {
	bk := Metadata{}

	pdf, err := pdf_parser.ParsePdf(file)

	if err != nil {
		return bk, err
	}

	title := pdf.GetTitle()
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	}

	year := ""
	date, err := time.Parse("2006-01-02T15:04:05-0700", pdf.GetDate())
	if err == nil {
		year = date.Format("2006")
	}

	description := pdf.GetDescription()
	if description != "" {
		p := bluemonday.UGCPolicy()
		description = p.Sanitize(description)
	}

	bk = Metadata{
		Title:       title,
		Author:      pdf.GetAuthor(),
		Description: template.HTML(description),
		Language:    pdf.GetLanguage(),
		Year:        year,
		Pages:       pdf.GetPagesCount(),
	}

	return bk, nil
}

// Cover parses the document looking for a cover image and returns it
func (p PdfReader) Cover(documentFullPath string, coverMaxWidth int) ([]byte, error) {
	var cover []byte

	return cover, fmt.Errorf("no cover available")
}
