package document

import (
	"errors"
	"log"
	"net/mail"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v4/internal/index"
)

func (d *Controller) Send(c fiber.Ctx) error {
	slug := c.Params("slug")

	if _, err := mail.ParseAddress(c.FormValue("email")); err != nil {
		return fiber.ErrBadRequest
	}

	file, err := d.idx.File(slug)
	if errors.Is(err, index.ErrDocumentNotFound) {
		return fiber.ErrNotFound
	} else if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	return d.sender.SendDocument(c.FormValue("email"), file.Document.Title, file.Data, file.FileName)
}
