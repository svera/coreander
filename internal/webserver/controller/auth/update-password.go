package auth

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v3/internal/webserver/infrastructure"
	"github.com/svera/coreander/v3/internal/webserver/model"
)

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

	if user.RecoveryValidUntil.Before(time.Now()) {
		return user, fiber.ErrBadRequest
	}

	return user, nil
}
