package document

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/pgaskin/kepubify/v4/kepub"
)

func (d *Controller) Download(c fiber.Ctx) error {
	slug := c.Params("slug")

	result, err := d.idx.File(slug)
	if err != nil {
		return fiber.ErrNotFound
	}

	data := result.Data
	fileName := result.FileName
	contentType := result.ContentType

	if strings.ToLower(c.Query("format")) == "kepub" && result.ContentType == "application/epub+zip" {
		z, err := zip.NewReader(bytes.NewReader(result.Data), int64(len(result.Data)))
		if err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
		buf := bytes.NewBuffer(nil)
		if err := kepub.NewConverter().Convert(context.Background(), buf, z); err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
		data = buf.Bytes()
		fileName = strings.TrimSuffix(filepath.Base(result.FileName), filepath.Ext(result.FileName)) + ".kepub.epub"
	}

	c.Response().Header.Set(fiber.HeaderContentType, contentType)
	c.Response().Header.Set(fiber.HeaderContentDisposition, fmt.Sprintf("inline; filename=\"%s\"", fileName))
	c.Response().BodyWriter().Write(data)
	return nil
}
