package metadata

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/flotzilla/pdf_parser"
	"github.com/gofiber/fiber/v2"
	"github.com/microcosm-cc/bluemonday"
	"github.com/sunshineplan/imgconv"
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
		Type:        "PDF",
	}

	return bk, nil
}

// Cover parses the document looking for a cover image and returns it
func (p PdfReader) Cover(documentFullPath string, coverMaxWidth int) ([]byte, error) {
	src, err := imgconv.Open(documentFullPath)
	if err != nil {
		log.Fatalf("failed to open image: %v", err)
	}
	dst := imaging.Resize(src, coverMaxWidth, 0, imaging.Box)
	if err != nil {
		return nil, fiber.ErrInternalServerError
	}

	buf := new(bytes.Buffer)
	err = imaging.Encode(buf, dst, imaging.JPEG)
	if err != nil {
		return []byte{}, fmt.Errorf("no cover available")
	}

	return buf.Bytes(), nil
}
