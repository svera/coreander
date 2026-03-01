package document

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/pgaskin/kepubify/v4/kepub"
)

func (d *Controller) Download(c fiber.Ctx) error {
	var (
		output   []byte
		err      error
		fileName string
	)

	document, err := d.idx.Document(c.Params("slug"))
	if err != nil {
		return fiber.ErrBadRequest
	}

	fullPath := filepath.Join(d.config.LibraryPath, document.ID)

	if _, err := os.Stat(fullPath); err != nil {
		return fiber.ErrNotFound
	}

	if strings.ToLower(c.Query("format")) == "kepub" {
		output, err = kepubify(fullPath)
		if err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
		fileName = strings.TrimSuffix(filepath.Base(fullPath), filepath.Ext(fullPath))
		fileName = fileName + ".kepub.epub"
	} else {
		file, err := os.Open(fullPath)
		if err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
		if output, err = io.ReadAll(file); err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
		fileName = filepath.Base(document.ID)
	}

	ext := strings.ToLower(filepath.Ext(document.ID))

	if ext == ".epub" {
		c.Response().Header.Set(fiber.HeaderContentType, "application/epub+zip")
	} else {
		c.Response().Header.Set(fiber.HeaderContentType, "application/pdf")
	}

	c.Response().Header.Set(fiber.HeaderContentDisposition, fmt.Sprintf("inline; filename=\"%s\"", fileName))
	c.Response().BodyWriter().Write(output)
	return nil
}

func kepubify(fullPath string) ([]byte, error) {
	output := bytes.NewBuffer(nil)
	r, err := zip.OpenReader(fullPath)
	if err != nil {
		return nil, fiber.ErrInternalServerError
	}
	defer r.Close()

	if err = kepub.NewConverter().Convert(context.Background(), output, r); err != nil {
		return nil, fiber.ErrInternalServerError
	}

	return output.Bytes(), nil
}
