package auth

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
)

func (a *Controller) Login(c *fiber.Ctx) error {
	resetPassword := fmt.Sprintf(
		"%s/reset-password",
		c.Locals("fqdn").(string),
	)

	msg := ""
	if ref := string(c.Request().Header.Referer()); strings.HasPrefix(ref, resetPassword) {
		msg = "Password changed successfully. Please sign in."
	}

	emailSendingConfigured := true
	if _, ok := a.sender.(*infrastructure.NoEmail); ok {
		emailSendingConfigured = false
	}

	return c.Render("auth/login", fiber.Map{
		"Title":                  "Login",
		"Success":                msg,
		"EmailSendingConfigured": emailSendingConfigured,
		"DisableLoginLink":       true,
	}, "layout")
}
