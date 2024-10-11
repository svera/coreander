package auth

import (
	"fmt"
	"net/mail"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
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
		user.RecoveryValidUntil = time.Now().UTC().Add(a.config.RecoveryTimeout)
		if err := a.repository.Update(user); err != nil {
			return fiber.ErrInternalServerError
		}

		recoveryLink := fmt.Sprintf(
			"%s/reset-password?id=%s",
			c.Locals("fqdn"),
			user.RecoveryUUID,
		)
		c.Render("auth/email", fiber.Map{
			"RecoveryLink":    recoveryLink,
			"RecoveryTimeout": strconv.FormatFloat(a.config.RecoveryTimeout.Hours(), 'f', -1, 64),
		})

		a.sender.Send(
			c.FormValue("email"),
			a.printers[c.Locals("Lang").(string)].Sprintf("Password recovery request"),
			string(c.Response().Body()),
		)
	}

	return c.Render("auth/request", fiber.Map{
		"Title":  "Recover password",
		"Errors": map[string]string{},
	}, "layout")
}
