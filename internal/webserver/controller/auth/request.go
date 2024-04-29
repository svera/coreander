package auth

import (
	"fmt"
	"net/mail"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/svera/coreander/v3/internal/webserver/infrastructure"
)

func (a *Controller) Request(c *fiber.Ctx) error {
	if _, ok := a.sender.(*infrastructure.NoEmail); ok {
		return fiber.ErrNotFound
	}

	if _, err := mail.ParseAddress(c.FormValue("email")); err != nil {
		return c.Render("auth/recover", fiber.Map{
			"Title":  "Recover password",
			"Errors": map[string]string{"email": "Incorrect email address"},
		}, "layout")
	}

	user, err := a.repository.FindByEmail(c.FormValue("email"))
	if err != nil {
		return fiber.ErrInternalServerError
	}

	if user != nil {
		user.RecoveryUUID = uuid.NewString()
		user.RecoveryValidUntil = time.Now().Add(a.config.SessionTimeout)
		if err := a.repository.Update(user); err != nil {
			return fiber.ErrInternalServerError
		}

		recoveryLink := fmt.Sprintf(
			"%s/%s/reset-password?id=%s",
			c.Locals("fqdn"),
			c.Params("lang"),
			user.RecoveryUUID,
		)
		c.Render("auth/email", fiber.Map{
			"Lang":         c.Params("lang"),
			"RecoveryLink": recoveryLink,
		})

		go a.sender.Send(
			c.FormValue("email"),
			a.printers[c.Params("lang")].Sprintf("Password recovery request"),
			string(c.Response().Body()),
		)
	}

	return c.Render("auth/request", fiber.Map{
		"Title":  "Recover password",
		"Errors": map[string]string{},
	}, "layout")
}
