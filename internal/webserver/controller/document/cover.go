package document

import (
	"fmt"
	"log"

	"github.com/gofiber/fiber/v3"
)

func (d *Controller) Cover(c fiber.Ctx) error {
	// Set cache control headers
	cacheControl := fmt.Sprintf("public, max-age=%d", d.config.ClientImageCacheTTL)
	c.Set("Cache-Control", cacheControl)
	c.Append("Cache-Time", fmt.Sprintf("%d", d.config.ServerImageCacheTTL))

	image, err := d.idx.Cover(c.Params("slug"), d.config.CoverMaxWidth)
	if err != nil {
		log.Println(err)
		return fiber.ErrNotFound
	}

	c.Response().Header.Set(fiber.HeaderContentType, "image/jpeg")
	c.Response().BodyWriter().Write(image)
	return nil
}
