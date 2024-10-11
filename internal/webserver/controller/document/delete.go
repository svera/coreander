package document

import (
	"log"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
)

func (d *Controller) Delete(c *fiber.Ctx) error {
	document, err := d.idx.Document(c.Params("slug"))
	if err != nil {
		return fiber.ErrBadRequest
	}

	fullPath := filepath.Join(d.config.LibraryPath, document.ID)
	if _, err := d.appFs.Stat(fullPath); err != nil {
		return fiber.ErrBadRequest
	}

	if err := d.idx.RemoveFile(fullPath); err != nil {
		return fiber.ErrInternalServerError
	}

	if err := d.appFs.Remove(fullPath); err != nil {
		log.Printf("error removing file %s", fullPath)
	}

	if err := d.hlRepository.RemoveDocument(document.ID); err != nil {
		log.Printf("error removing file %s from highlights", document.ID)
	}

	return nil
}
