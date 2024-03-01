package auth

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v3/internal/webserver/controller"
	"github.com/svera/coreander/v3/internal/webserver/infrastructure"
)

func (a *Controller) Login(c *fiber.Ctx) error {
	resetPassword := fmt.Sprintf(
		"%s://%s%s/%s/reset-password",
		c.Protocol(),
		a.config.Hostname,
		controller.UrlPort(c.Protocol(), a.config.Port),
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
