package user

import (
	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v3/internal/webserver/model"
)

// Delete removes a user from the database
func (u *Controller) Delete(c *fiber.Ctx) error {
	user, err := u.repository.FindByUuid(c.FormValue("uuid"))
	if err != nil {
		return fiber.ErrInternalServerError
	}

	if user == nil {
		return fiber.ErrNotFound
	}

	if u.repository.Admins() == 1 && user.Role == model.RoleAdmin {
		return fiber.ErrForbidden
	}

	if err = u.repository.Delete(c.FormValue("uuid")); err != nil {
		return fiber.ErrInternalServerError
	}

	return nil
}
