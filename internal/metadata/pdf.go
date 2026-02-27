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

	"github.com/flotzilla/pdf_parser"
	"github.com/hhrutter/tiff"
	"github.com/microcosm-cc/bluemonday"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/rickb777/date/v2"
	"github.com/svera/coreander/v4/internal/precisiondate"
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

	publication := precisiondate.PrecisionDate{Precision: precisiondate.PrecisionDay}
	if publication.Date, err = date.Parse("2006-01-02", pdf.GetDate()); err != nil {
		publication.Precision = precisiondate.PrecisionYear
		publication.Date, _ = date.Parse("2006", pdf.GetDate())
	}

	description := pdf.GetDescription()
	if description != "" {
		p := bluemonday.UGCPolicy()
		description = p.Sanitize(description)
	}

	authors := []string{""}
	if pdf.GetAuthor() != "" {
		// We want to identify cases with multiple authors looking for specific separators and then indexing each author properly.
		authors = strings.Split(pdf.GetAuthor(), "&")
		for i := range authors {
			authors[i] = strings.TrimSpace(authors[i])
		}
	}

	lang := pdf.GetLanguage()

	bk = Metadata{
		Title:       title,
		Authors:     authors,
		Description: template.HTML(description),
		Language:    lang,
		Publication: publication,
		Pages:       float64(pdf.GetPagesCount()),
		Format:      "PDF",
		Subjects:    []string{},
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

// Illustrations returns the number of illustrations; PDFs are not counted for now.
func (p PdfReader) Illustrations(documentFullPath string, minMegapixels float64) (int, error) {
	return 0, nil
}

func decodePDF(r io.Reader) (io.Reader, error) {
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed

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
