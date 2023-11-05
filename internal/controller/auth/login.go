package auth

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v3/internal/infrastructure"
)

func (a *Controller) Login(c *fiber.Ctx) error {
	resetPassword := fmt.Sprintf(
		"%s://%s%s/%s/reset-password",
		c.Protocol(),
		a.config.Hostname,
		a.urlPort(c),
		c.Params("lang"),
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
		"Message":                msg,
		"EmailSendingConfigured": emailSendingConfigured,
	}, "layout")
}