package auth

import (
	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/infrastructure"
)

func (a *Controller) Recover(c *fiber.Ctx) error {
	if _, ok := a.sender.(*infrastructure.NoEmail); ok {
		return fiber.ErrNotFound
	}

	return c.Render("auth/recover", fiber.Map{
		"Title":  "Recover password",
		"Errors": map[string]string{},
	}, "layout")
}
