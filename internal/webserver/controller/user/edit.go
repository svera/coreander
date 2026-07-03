package user

import (
	"log"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v5/internal/webserver/model"
)

// buildEditUserVars assembles the template vars shared by the edit form's initial
// render and its post-update re-render, so both stay in sync (e.g. IsLastAdmin).
func (u *Controller) buildEditUserVars(user *model.User, activeTab string, errs map[string]string) fiber.Map {
	if errs == nil {
		errs = map[string]string{}
	}
	return fiber.Map{
		"Title":              "Edit user",
		"User":               user,
		"MinPasswordLength":  u.config.MinPasswordLength,
		"UsernamePattern":    model.UsernamePattern,
		"Errors":             errs,
		"EmailFrom":          u.sender.From(),
		"ActiveTab":          activeTab,
		"IsLastAdmin":        u.usersRepository.IsLastAdmin(user),
	}
}

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

	vars := u.buildEditUserVars(user, "options", nil)
	vars["AvailableLanguages"] = c.Locals("AvailableLanguages")

	if c.Get("HX-Request") == "true" {
		return c.Render("user/edit", vars)
	}

	return c.Render("user/edit", vars, "layout")
}
