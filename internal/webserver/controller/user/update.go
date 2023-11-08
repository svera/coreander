package user

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/model"
	"github.com/svera/coreander/v4/internal/webserver/jwtclaimsreader"
)

// Update gathers information from the edit user form and updates user data
func (u *Controller) Update(c *fiber.Ctx) error {
	user, err := u.repository.FindByUuid(c.Params("uuid"))
	if err != nil {
		return fiber.ErrNotFound
	}

	session := jwtclaimsreader.SessionData(c)

	if session.Role != model.RoleAdmin && session.Uuid != c.Params("uuid") {
		return fiber.ErrForbidden
	}

	if c.FormValue("password-tab") == "true" {
		return u.updatePassword(c, session, *user)
	}

	user.Name = c.FormValue("name")
	user.SendToEmail = c.FormValue("send-to-email")
	user.WordsPerMinute, _ = strconv.ParseFloat(c.FormValue("words-per-minute"), 64)

	errs := user.Validate(u.config.MinPasswordLength)
	if len(errs) > 0 {
		return c.Render("users/edit", fiber.Map{
			"Title":             "Edit user",
			"User":              user,
			"Session":           session,
			"MinPasswordLength": u.config.MinPasswordLength,
			"Errors":            errs,
		}, "layout")
	}

	if err := u.repository.Update(user); err != nil {
		return fiber.ErrInternalServerError
	}

	return c.Render("users/edit", fiber.Map{
		"Title":             "Edit user",
		"User":              user,
		"Session":           session,
		"MinPasswordLength": u.config.MinPasswordLength,
		"Errors":            errs,
		"Message":           "Profile updated",
	}, "layout")
}

// updatePassword gathers information from the edit user form and updates user password
func (u *Controller) updatePassword(c *fiber.Ctx, session, user model.User) error {
	user.Password = c.FormValue("password")

	errs := user.Validate(u.config.MinPasswordLength)

	// Allow admins to change password of other users without entering user's current password
	if session.Uuid == c.Params("uuid") {
		user, err := u.repository.FindByEmail(user.Email)
		if err != nil {
			return fiber.ErrInternalServerError
		}

		if user.Password != model.Hash(c.FormValue("old-password")) {
			errs["oldpassword"] = "The current password is not correct"
		}
	}

	if errs = user.ConfirmPassword(c.FormValue("confirm-password"), u.config.MinPasswordLength, errs); len(errs) > 0 {
		return c.Render("users/edit", fiber.Map{
			"Title":             "Edit user",
			"User":              user,
			"Session":           session,
			"MinPasswordLength": u.config.MinPasswordLength,
			"ActiveTab":         "password",
			"Errors":            errs,
		}, "layout")
	}

	user.Password = model.Hash(user.Password)
	if err := u.repository.Update(&user); err != nil {
		return fiber.ErrInternalServerError
	}

	return c.Render("users/edit", fiber.Map{
		"Title":             "Edit user",
		"User":              user,
		"Session":           session,
		"MinPasswordLength": u.config.MinPasswordLength,
		"ActiveTab":         "password",
		"Errors":            errs,
		"Message":           "Password updated",
	}, "layout")
}
