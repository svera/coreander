package highlight

import (
	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/jwtclaimsreader"
)

func (h *Controller) Remove(c *fiber.Ctx) error {
	session := jwtclaimsreader.SessionData(c)

	user, err := h.usrRepository.FindByUuid(session.Uuid)
	if err != nil {
		return fiber.ErrBadRequest
	}

	document, err := h.idx.Document(c.FormValue("slug"))
	if err != nil {
		return fiber.ErrBadRequest
	}

	return h.hlRepository.Remove(int(user.ID), document.ID)
}
