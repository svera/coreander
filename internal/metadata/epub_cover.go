package metadata

import (
	"archive/zip"
	"bytes"
	"fmt"

	"github.com/disintegration/imaging"
	"github.com/gofiber/fiber/v2"
)

// Cover parses the book looking for a cover image, and extracts it to outputFolder
func (e EpubReader) Cover(bookFullPath string, coverMaxWidth int) ([]byte, error) {
	var cover []byte

	reader := EpubReader{}
	meta, err := reader.Metadata(bookFullPath)
	if err != nil {
		return nil, err
	}
	if meta.Cover == "" {
		return nil, fmt.Errorf("no cover image set in %s", bookFullPath)
	}

	r, err := zip.OpenReader(bookFullPath)
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
		dst := imaging.Resize(src, coverMaxWidth, 0, imaging.Box)
		if err != nil {
			return nil, fiber.ErrInternalServerError
		}

		buf := new(bytes.Buffer)
		imaging.Encode(buf, dst, imaging.JPEG)
		return buf.Bytes(), nil
	}
	return nil, fmt.Errorf("no cover image found")
}
