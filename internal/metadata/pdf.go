package metadata

import (
	"bytes"
	"fmt"
	"html/template"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/flotzilla/pdf_parser"
	"github.com/hhrutter/tiff"
	"github.com/microcosm-cc/bluemonday"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
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
		Authors:     []string{pdf.GetAuthor()},
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
	f, err := os.ReadFile(documentFullPath)
	if err != nil {
		return nil, err
	}
	pr, err := decodePDF(bytes.NewBuffer(f))
	if err != nil {
		return nil, err
	}

	src, err := decodeImage(pr)
	if err != nil {
		return nil, err
	}

	return resize(src, coverMaxWidth, err)
}

func decodePDF(r io.Reader) (io.Reader, error) {
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationNone

	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	ctx, err := pdfcpu.Read(bytes.NewReader(b), conf)
	if err != nil {
		return nil, err
	}

	if err := api.OptimizeContext(ctx); err != nil {
		return nil, err
	}
	if ctx.PageCount == 0 {
		return nil, fmt.Errorf("page count is zero")
	}

	for p := 1; p <= ctx.PageCount; p++ {
		imgs, err := pdfcpu.ExtractPageImages(ctx, p, false)
		if err != nil {
			return nil, err
		}

		for _, img := range imgs {
			if img.Reader != nil {
				return img, nil
			}
		}
	}
	return nil, fmt.Errorf("no image found")
}

func decodeImage(r io.Reader) (image.Image, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	img, format, err := image.Decode(bytes.NewBuffer(b))
	if format == "tiff" && err != nil {
		return tiff.Decode(bytes.NewBuffer(b))
	}

	return img, err
}
