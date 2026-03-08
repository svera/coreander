package metadata

import (
	"bytes"
	"fmt"
	"html/template"
	"image"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hhrutter/tiff"
	"github.com/microcosm-cc/bluemonday"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/rickb777/date/v2"
	"github.com/svera/coreander/v4/internal/precisiondate"
)

type PdfReader struct{}

// pdfDateRe matches PDF date format D:YYYYMMDD... and captures YYYY, MM, DD.
var pdfDateRe = regexp.MustCompile(`D:(\d{4})(\d{2})?(\d{2})?`)

func (p PdfReader) Metadata(file string) (Metadata, error) {
	bk := Metadata{}

	f, err := os.ReadFile(file)
	if err != nil {
		return bk, err
	}

	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	info, err := api.PDFInfo(bytes.NewReader(f), filepath.Base(file), nil, false, conf)
	if err != nil {
		return bk, err
	}

	title := strings.TrimSpace(info.Title)
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	}

	publication := precisiondate.PrecisionDate{Precision: precisiondate.PrecisionDay}
	dateStr := normalizePDFDate(info.CreationDate, info.ModificationDate)
	if publication.Date, err = date.Parse("2006-01-02", dateStr); err != nil {
		publication.Precision = precisiondate.PrecisionYear
		publication.Date, _ = date.Parse("2006", dateStr)
	}

	description := strings.TrimSpace(info.Subject)
	if description != "" {
		policy := bluemonday.UGCPolicy()
		description = policy.Sanitize(description)
	}

	authors := []string{""}
	if author := strings.TrimSpace(info.Author); author != "" {
		authors = strings.Split(author, "&")
		for i := range authors {
			authors[i] = strings.TrimSpace(authors[i])
		}
	}

	lang := ""
	if info.Properties != nil {
		if l, ok := info.Properties["Language"]; ok {
			lang = strings.TrimSpace(l)
		}
	}

	illustrations, err := p.Illustrations(file, 0.25)
	if err != nil {
		log.Printf("Cannot count illustrations in %s: %s\n", file, err)
	}

	bk = Metadata{
		Title:         title,
		Authors:       authors,
		Description:   template.HTML(description),
		Language:      lang,
		Publication:   publication,
		Pages:         float64(info.PageCount),
		Format:        "PDF",
		Subjects:      []string{},
		Illustrations: illustrations,
	}

	return bk, nil
}

// normalizePDFDate returns a date string suitable for date.Parse, preferring creation then modification.
// PDF dates are typically like "D:20210101120000+00'00'"; we extract YYYY-MM-DD or YYYY when possible.
func normalizePDFDate(creation, modification string) string {
	for _, raw := range []string{creation, modification} {
		if raw == "" {
			continue
		}
		if m := pdfDateRe.FindStringSubmatch(raw); len(m) >= 2 {
			year := m[1]
			if len(m) >= 4 && m[2] != "" && m[3] != "" {
				return year + "-" + m[2] + "-" + m[3]
			}
			return year
		}
	}
	return ""
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

// Illustrations returns the number of distinct embedded images with pixel count >= minMegapixels.
func (p PdfReader) Illustrations(documentFullPath string, minMegapixels float64) (int, error) {
	f, err := os.ReadFile(documentFullPath)
	if err != nil {
		return 0, err
	}
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	ctx, err := pdfcpu.Read(bytes.NewReader(f), conf)
	if err != nil {
		return 0, err
	}
	// Ensure page count is set (pdfcpu may leave it 0 when validation is skipped or for some PDFs).
	if err := ctx.EnsurePageCount(); err != nil {
		log.Printf("Cannot get page count for %s: %v", documentFullPath, err)
		return 0, nil
	}
	if err := api.OptimizeContext(ctx); err != nil {
		return 0, err
	}
	if ctx.PageCount == 0 {
		return 0, nil
	}
	seen := make(map[int]struct{})
	var count int
	for pageNr := 1; pageNr <= ctx.PageCount; pageNr++ {
		imgs, err := pdfcpu.ExtractPageImages(ctx, pageNr, true)
		if err != nil {
			return 0, err
		}
		for _, img := range imgs {
			if _, counted := seen[img.ObjNr]; counted {
				continue
			}
			mp := float64(img.Width*img.Height) / 1e6
			if img.Width > 0 && img.Height > 0 && mp >= minMegapixels {
				seen[img.ObjNr] = struct{}{}
				count++
			}
		}
	}
	return count, nil
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
	// Ensure page count is set (pdfcpu may leave it 0 when validation is skipped or for some PDFs).
	if err := ctx.EnsurePageCount(); err != nil {
		return nil, fmt.Errorf("no image found")
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
