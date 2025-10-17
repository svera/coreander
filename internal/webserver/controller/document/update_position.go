package document

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

func (d *Controller) UpdatePosition(c *fiber.Ctx) error {
	document, err := d.idx.Document(c.Params("slug"))
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	if document.Slug == "" {
		return fiber.ErrNotFound
	}

	var session model.Session
	if val, ok := c.Locals("Session").(model.Session); ok {
		session = val
	}

	if session.ID == 0 {
		return fiber.ErrUnauthorized
	}

	var body struct {
		CFI string `json:"cfi"`
	}

	if err := c.BodyParser(&body); err != nil {
		return fiber.ErrBadRequest
	}

	if err := d.readingRepository.Update(int(session.ID), document.ID, body.CFI); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	return c.SendStatus(fiber.StatusNoContent)
}
