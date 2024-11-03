package document

import (
	"errors"
	"log"
	"os"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
)

func (d *Controller) Delete(c *fiber.Ctx) error {
	document, err := d.idx.Document(c.Params("slug"))
	if err != nil {
		return fiber.ErrBadRequest
	}

	fullPath := filepath.Join(d.config.LibraryPath, document.ID)
	if _, err := d.appFs.Stat(fullPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("error checking file %s for removal: %s\n", fullPath, err.Error())
		return fiber.ErrBadRequest
	}

	if err := d.idx.RemoveFile(fullPath); err != nil {
		log.Printf("error removing file %s from index: %s\n", fullPath, err.Error())
		return fiber.ErrInternalServerError
	}

	if err := d.appFs.Remove(fullPath); err != nil {
		log.Printf("error removing file %s\n", fullPath)
	}

	if err := d.hlRepository.RemoveDocument(document.ID); err != nil {
		log.Printf("error removing file %s from highlights\n", document.ID)
	}

	return nil
}
