package user

import (
	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

// Delete removes a user from the database. Admins may delete other users. A signed-in user may delete only their own account, after submitting confirm-username matching their username exactly.
func (u *Controller) Delete(c fiber.Ctx) error {
	var session model.Session
	if val, ok := c.Locals("Session").(model.Session); ok {
		session = val
	}

	user, err := u.usersRepository.FindByUsername(c.Params("username"))
	if err != nil {
		return fiber.ErrInternalServerError
	}

	if user == nil {
		return fiber.ErrNotFound
	}

	isSelf := session.Username == user.Username
	isAdmin := session.Role == model.RoleAdmin

	if !isSelf && !isAdmin {
		return fiber.ErrForbidden
	}

	// Never delete the last admin (including self-service); must keep at least one admin account.
	if user.Role == model.RoleAdmin && u.usersRepository.Admins() == 1 {
		return fiber.ErrForbidden
	}

	if isSelf && session.Role != model.RoleAdmin {
		if c.FormValue("confirm-username") != user.Username {
			return fiber.ErrBadRequest
		}
	}

	if err = u.usersRepository.Delete(user.Uuid); err != nil {
		return fiber.ErrInternalServerError
	}

	if isSelf {
		c.Cookie(&fiber.Cookie{
			Name:     "session",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			Secure:   false,
			HTTPOnly: true,
		})
		c.Set("HX-Redirect", "/")
		return c.SendStatus(fiber.StatusNoContent)
	}

	return nil
}
