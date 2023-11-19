package user

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/webserver/jwtclaimsreader"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

// Delete removes a user from the database
func (u *Controller) Delete(c *fiber.Ctx) error {
	session := jwtclaimsreader.SessionData(c)

	if session.Role != model.RoleAdmin {
		return fiber.ErrForbidden
	}

	user, err := u.repository.FindByUuid(c.FormValue("uuid"))
	if err != nil {
		return fiber.ErrNotFound
	}
	if u.repository.Admins() == 1 && user.Role == model.RoleAdmin {
		return fiber.ErrForbidden
	}

	u.repository.Delete(c.FormValue("uuid"))
	return c.Redirect(fmt.Sprintf("/%s/users", c.Params("lang")))
}
