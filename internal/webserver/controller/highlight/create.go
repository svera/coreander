package highlight

import (
	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

func (h *Controller) Create(c *fiber.Ctx) error {
	user := c.Locals("Session").(model.Session)

	document, err := h.idx.Document(c.Params("slug"))
	if err != nil {
		return fiber.ErrBadRequest
	}

	return h.hlRepository.Highlight(int(user.ID), document.ID)
}
