package auth

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

func (a *Controller) EditPassword(c *fiber.Ctx) error {
	if _, err := a.validateRecoveryAccess(c.Query("id")); err != nil {
		return err
	}

	return c.Render("auth/edit-password", fiber.Map{
		"Title":  "Reset password",
		"Uuid":   c.Query("id"),
		"Errors": map[string]string{},
	}, "layout")
}

func (a *Controller) UpdatePassword(c *fiber.Ctx) error {
	user, err := a.validateRecoveryAccess(c.FormValue("id"))
	if err != nil {
		return err
	}

	user.Password = c.FormValue("password")
	user.RecoveryUUID = ""
	user.RecoveryValidUntil = time.Unix(0, 0)
	errs := map[string]string{}

	if errs = user.ConfirmPassword(c.FormValue("confirm-password"), a.config.MinPasswordLength, errs); len(errs) > 0 {
		return c.Render("auth/edit-password", fiber.Map{
			"Title":  "Reset password",
			"Uuid":   c.FormValue("id"),
			"Errors": errs,
		}, "layout")
	}

	user.Password = model.Hash(user.Password)
	if err := a.repository.Update(user); err != nil {
		return fiber.ErrInternalServerError
	}

	return c.Redirect(fmt.Sprintf("/%s/login", c.Params("lang")))
}

func (a *Controller) validateRecoveryAccess(recoveryUuid string) (*model.User, error) {
	if _, ok := a.sender.(*infrastructure.NoEmail); ok {
		return &model.User{}, fiber.ErrNotFound
	}

	if recoveryUuid == "" {
		return &model.User{}, fiber.ErrBadRequest
	}
	user, err := a.repository.FindByRecoveryUuid(recoveryUuid)
	if err != nil {
		return user, fiber.ErrInternalServerError
	}

	if user.RecoveryValidUntil.UTC().After(time.Now().UTC()) {
		return user, nil
	}

	return user, fiber.ErrBadRequest

}
