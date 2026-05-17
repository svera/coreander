package document

import (
	"log"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

type updateReadingPositionBody struct {
	Position   string `json:"position"`
	Percentage int    `json:"percentage"`
}

func (d *Controller) UpdatePosition(c fiber.Ctx) error {
	document, err := d.idx.Document(c.Params("slug"))
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	if document.Slug == "" {
		return fiber.ErrNotFound
	}

	session, _ := c.Locals("Session").(model.Session)

	var body updateReadingPositionBody
	if err := c.Bind().Body(&body); err != nil {
		return fiber.ErrBadRequest
	}

	pct := model.ClampReadingPercentage(body.Percentage)
	if err := d.readingRepository.Update(int(session.ID), document.Slug, body.Position, pct); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	return c.SendStatus(fiber.StatusNoContent)
}
