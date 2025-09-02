package auth

import (
	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
)

func (a *Controller) Login(c *fiber.Ctx) error {
	emailSendingConfigured := true
	if _, ok := a.sender.(*infrastructure.NoEmail); ok {
		emailSendingConfigured = false
	}

	return c.Render("auth/login", fiber.Map{
		"Title":                  a.translator.T(c.Locals("Lang").(string), "Log in"),
		"EmailSendingConfigured": emailSendingConfigured,
		"DisableLoginLink":       true,
	}, "layout")
}
