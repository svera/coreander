package document

import (
	"log"
	"net/mail"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func (d *Controller) Send(c *fiber.Ctx) error {
	slug := ""
	if slug = strings.Trim(c.Params("slug"), " "); slug == "" {
		return fiber.ErrBadRequest
	}

	if _, err := mail.ParseAddress(c.FormValue("email")); err != nil {
		return fiber.ErrBadRequest
	}

	document, err := d.idx.Document(slug)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if _, err := os.Stat(filepath.Join(d.config.LibraryPath, document.ID)); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	return d.sender.SendDocument(c.FormValue("email"), document.Title, d.config.LibraryPath, document.ID)
}
