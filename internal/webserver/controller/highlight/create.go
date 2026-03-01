package highlight

import (
	"log"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

func (h *Controller) Create(c fiber.Ctx) error {
	user := c.Locals("Session").(model.Session)

	document, err := h.idx.Document(c.Params("slug"))
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err = h.hlRepository.Highlight(int(user.ID), document.ID); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	c.Response().Header.Set("HX-Trigger", "highlight")
	return nil
}
