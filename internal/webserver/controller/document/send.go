package document

import (
	"log"
	"net/mail"

	"github.com/gofiber/fiber/v3"
)

func (d *Controller) Send(c fiber.Ctx) error {
	slug := c.Params("slug")

	if _, err := mail.ParseAddress(c.FormValue("email")); err != nil {
		return fiber.ErrBadRequest
	}

	document, err := d.idx.Document(slug)
	if err != nil {
		return fiber.ErrInternalServerError
	}

	if document.Slug == "" {
		return fiber.ErrNotFound
	}

	file, err := d.idx.File(slug)
	if err != nil {
		log.Println(err)
		return fiber.ErrNotFound
	}

	return d.sender.SendDocument(c.FormValue("email"), document.Title, file.Data, file.FileName)
}
