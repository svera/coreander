package document

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
)

func (d *Controller) Delete(c *fiber.Ctx) error {
	if c.FormValue("slug") == "" {
		return fiber.ErrBadRequest
	}

	document, err := d.idx.Document(c.FormValue("slug"))
	if err != nil {
		fmt.Println(err)
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

	return nil
}
