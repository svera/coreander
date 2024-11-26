package highlight

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

func (h *Controller) Delete(c *fiber.Ctx) error {
	user := c.Locals("Session").(model.Session)

	document, err := h.idx.Document(c.Params("slug"))
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err = h.hlRepository.Remove(int(user.ID), document.ID); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	c.Response().Header.Set("HX-Trigger", "dehighlight")
	return nil
}
