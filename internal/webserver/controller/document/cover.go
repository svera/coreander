package document

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
)

func (d *Controller) Cover(c *fiber.Ctx) error {
	// Set cache control headers
	cacheControl := fmt.Sprintf("public, max-age=%d", d.config.ImageCacheTTL)
	c.Set("Cache-Control", cacheControl)
	c.Append("Cache-Time", fmt.Sprintf("%d", d.config.ImageCacheTTL))

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
