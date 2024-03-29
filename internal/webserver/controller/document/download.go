package document

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func (d *Controller) Download(c *fiber.Ctx) error {
	document, err := d.idx.Document(c.Params("slug"))
	if err != nil {
		return fiber.ErrBadRequest
	}

	fullPath := filepath.Join(d.config.LibraryPath, document.ID)

	if _, err := os.Stat(fullPath); err != nil {
		return fiber.ErrNotFound
	}

	file, err := os.Open(fullPath)
	if err != nil {
		return fiber.ErrInternalServerError
	}
	contents, err := io.ReadAll(file)
	if err != nil {
		return fiber.ErrInternalServerError
	}

	ext := strings.ToLower(filepath.Ext(document.ID))

	if ext == ".epub" {
		c.Response().Header.Set(fiber.HeaderContentType, "application/epub+zip")
	} else {
		c.Response().Header.Set(fiber.HeaderContentType, "application/pdf")
	}

	c.Response().Header.Set(fiber.HeaderContentDisposition, fmt.Sprintf("inline; filename=\"%s\"", filepath.Base(document.ID)))
	c.Response().BodyWriter().Write(contents)
	return nil
}
