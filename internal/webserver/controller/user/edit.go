package user

import (
	"log"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

// Edit renders the edit user form
func (u *Controller) Edit(c fiber.Ctx) error {
	user, err := u.usersRepository.FindByUsername(c.Params("username"))
	if err != nil {
		log.Println(err.Error())
		return fiber.ErrInternalServerError
	}
	if user == nil {
		return fiber.ErrNotFound
	}

	var session model.Session
	if val, ok := c.Locals("Session").(model.Session); ok {
		session = val
	}

	if session.Role != model.RoleAdmin && session.Username != c.Params("username") {
		return fiber.ErrForbidden
	}

	vars := fiber.Map{
		"Title":              "Edit user",
		"User":               user,
		"MinPasswordLength":  u.config.MinPasswordLength,
		"UsernamePattern":    model.UsernamePattern,
		"Errors":             map[string]string{},
		"EmailFrom":          u.sender.From(),
		"ActiveTab":          "options",
		"AvailableLanguages": c.Locals("AvailableLanguages"),
	}

	if c.Get("HX-Request") == "true" {
		return c.Render("user/edit", vars)
	}

	return c.Render("user/edit", vars, "layout")
}
