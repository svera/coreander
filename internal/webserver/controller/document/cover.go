package document

import (
	"log"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
)

func (d *Controller) Cover(c *fiber.Ctx) error {
	// Set cache control headers with 24 hour TTL
	c.Set("Cache-Control", "public, max-age=86400")
	c.Append("Cache-Time", "86400")

	var (
		image []byte
	)

	document, err := d.idx.Document(c.Params("slug"))
	if err != nil {
		return fiber.ErrBadRequest
	}
	ext := filepath.Ext(document.ID)

	if _, ok := d.metadataReaders[ext]; !ok {
		return fiber.ErrBadRequest
	}
	image, err = d.metadataReaders[ext].Cover(filepath.Join(d.config.LibraryPath, document.ID), d.config.CoverMaxWidth)
	if err != nil {
		log.Println(err)
		return fiber.ErrNotFound
	}

	c.Response().Header.Set(fiber.HeaderContentType, "image/jpeg")
	c.Response().BodyWriter().Write(image)
	return nil
}
