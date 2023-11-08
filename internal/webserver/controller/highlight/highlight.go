package highlight

import (
	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v3/internal/webserver/jwtclaimsreader"
)

func (h *Controller) Highlight(c *fiber.Ctx) error {
	session := jwtclaimsreader.SessionData(c)

	user, err := h.usrRepository.FindByUuid(session.Uuid)
	if err != nil {
		return fiber.ErrBadRequest
	}

	document, err := h.idx.Document(c.FormValue("slug"))
	if err != nil {
		return fiber.ErrBadRequest
	}

	return h.hlRepository.Highlight(int(user.ID), document.ID)
}
