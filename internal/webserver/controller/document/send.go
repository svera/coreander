package document

import (
	"net/mail"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func (d *Controller) Send(c *fiber.Ctx) error {
	if strings.Trim(c.FormValue("slug"), " ") == "" {
		return fiber.ErrBadRequest
	}

	if _, err := mail.ParseAddress(c.FormValue("email")); err != nil {
		return fiber.ErrBadRequest
	}

	document, err := d.idx.Document(c.FormValue("slug"))
	if err != nil {
		return fiber.ErrBadRequest
	}

	if _, err := os.Stat(filepath.Join(d.config.LibraryPath, document.ID)); err != nil {
		return fiber.ErrBadRequest
	}

	return d.sender.SendDocument(c.FormValue("email"), d.config.LibraryPath, document.ID)
}
