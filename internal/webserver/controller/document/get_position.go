package document

import (
	"log"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

func (d *Controller) GetPosition(c fiber.Ctx) error {
	document, err := d.idx.Document(c.Params("slug"))
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	if document.Slug == "" {
		return fiber.ErrNotFound
	}

	session, _ := c.Locals("Session").(model.Session)

	reading, err := d.readingRepository.Get(int(session.ID), document.Slug)
	if err != nil {
		return c.JSON(fiber.Map{
			"position":   "",
			"updated":    "",
			"percentage": 0,
		})
	}

	return c.JSON(fiber.Map{
		"position":   reading.Position,
		"updated":    reading.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		"percentage": reading.Percentage,
	})
}
