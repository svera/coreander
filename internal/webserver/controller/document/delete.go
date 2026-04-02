package document

import (
	"errors"
	"log"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v4/internal/index"
)

func (d *Controller) Delete(c fiber.Ctx) error {
	slug := c.Params("slug")

	if err := d.idx.DeleteDocument(slug); err != nil {
		if errors.Is(err, index.ErrDocumentNotFound) {
			return fiber.ErrNotFound
		}
		return fiber.ErrInternalServerError
	}

	if err := d.hlRepository.RemoveDocument(slug); err != nil {
		log.Printf("error removing document %s from highlights\n", slug)
	}

	if err := d.readingRepository.RemoveDocument(slug); err != nil {
		log.Printf("error removing document %s from readings\n", slug)
	}

	return nil
}
