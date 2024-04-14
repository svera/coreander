package user

import (
	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v3/internal/webserver/model"
)

// New renders the new user form
func (u *Controller) New(c *fiber.Ctx) error {
	user := model.User{
		WordsPerMinute: u.config.WordsPerMinute,
	}
	return c.Render("users/new", fiber.Map{
		"Title":             "Add user",
		"MinPasswordLength": u.config.MinPasswordLength,
		"User":              user,
		"UsernamePattern":   model.UsernamePattern,
		"Errors":            map[string]string{},
	}, "layout")
}
