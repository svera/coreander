package highlight

import (
	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v3/internal/webserver/model"
)

func (h *Controller) Highlight(c *fiber.Ctx) error {
	user := c.Locals("Session").(model.User)

	document, err := h.idx.Document(c.FormValue("slug"))
	if err != nil {
		return fiber.ErrBadRequest
	}

	return h.hlRepository.Highlight(int(user.ID), document.ID)
}
