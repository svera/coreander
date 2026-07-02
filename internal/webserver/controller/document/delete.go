package document

import (
	"errors"
	"log"
	"os"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v5/internal/index"
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

	coverPath := d.config.CacheDir + "/covers/" + slug + ".webp"
	if err := d.appFs.Remove(coverPath); err != nil && !os.IsNotExist(err) {
		log.Printf("error removing cover cache for %s\n", slug)
	}

	return nil
}
