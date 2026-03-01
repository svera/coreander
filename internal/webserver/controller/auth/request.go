package auth

import (
	"fmt"
	"log"
	"net/mail"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
)

func (a *Controller) Request(c fiber.Ctx) error {
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
			log.Printf("error updating user with recovery UUID: %v\n", err)
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

		if err := a.sender.Send(
			c.FormValue("email"),
			a.translator.T(c.Locals("Lang").(string), "Password recovery request"),
			string(c.Response().Body()),
		); err != nil {
			log.Printf("error sending recovery email: %v\n", err)
			return fiber.ErrInternalServerError
		}
	}

	c.Cookie(&fiber.Cookie{
		Name:    "success-once",
		Value:   "<p>We've received your password recovery request. If the address you introduced is registered in our system, you'll receive an email with further instructions in your inbox.</p><p>Check your spam folder if you don't receive the recovery email after a while.</p>",
		Expires: time.Now().Add(24 * time.Hour),
	})
	return c.Redirect().To("/sessions/new")
}
