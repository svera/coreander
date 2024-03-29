package user

import (
	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v3/internal/webserver/model"
)

// Edit renders the edit user form
func (u *Controller) Edit(c *fiber.Ctx) error {
	user, err := u.repository.FindByUuid(c.Params("uuid"))
	if err != nil {
		return fiber.ErrNotFound
	}

	var session model.User
	if val, ok := c.Locals("Session").(model.User); ok {
		session = val
	}

	if session.Role != model.RoleAdmin && session.Uuid != c.Params("uuid") {
		return fiber.ErrForbidden
	}

	return c.Render("users/edit", fiber.Map{
		"Title":             "Edit user",
		"User":              user,
		"MinPasswordLength": u.config.MinPasswordLength,
		"Errors":            map[string]string{},
	}, "layout")
}
