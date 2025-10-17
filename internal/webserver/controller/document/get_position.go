package document

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

func (d *Controller) GetPosition(c *fiber.Ctx) error {
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

	reading, err := d.readingRepository.Get(int(session.ID), document.ID)
	if err != nil {
		// Return empty response if no position is stored
		return c.JSON(fiber.Map{
			"cfi":     "",
			"updated": "",
		})
	}

	return c.JSON(fiber.Map{
		"cfi":     reading.CFI,
		"updated": reading.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
	})
}
