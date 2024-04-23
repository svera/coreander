package user

import (
	"log"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v3/internal/webserver/controller/auth"
	"github.com/svera/coreander/v3/internal/webserver/model"
)

// Update gathers information from the edit user form and updates user data
func (u *Controller) Update(c *fiber.Ctx) error {
	user, err := u.repository.FindByUuid(c.FormValue("id"))
	if err != nil {
		log.Println(err.Error())
		return fiber.ErrInternalServerError
	}
	if user == nil {
		return fiber.ErrNotFound
	}

	var session model.User
	if val, ok := c.Locals("Session").(model.User); ok {
		session = val
	}

	if session.Role != model.RoleAdmin && user.Uuid != session.Uuid {
		return fiber.ErrForbidden
	}

	if c.FormValue("password-tab") == "true" {
		return u.updateUserPassword(c, session, *user)
	}

	return u.updateUserData(c, user, session)
}

func (u *Controller) updateUserData(c *fiber.Ctx, user *model.User, session model.User) error {
	user.Name = c.FormValue("name")
	user.Username = strings.ToLower(c.FormValue("username"))
	user.Email = c.FormValue("email")
	user.SendToEmail = c.FormValue("send-to-email")
	user.WordsPerMinute, _ = strconv.ParseFloat(c.FormValue("words-per-minute"), 64)

	validationErrs, err := u.validate(c, user, session)
	if err != nil {
		return err
	}

	if len(validationErrs) > 0 {
		return c.Render("users/edit", fiber.Map{
			"Title":             "Edit user",
			"User":              user,
			"MinPasswordLength": u.config.MinPasswordLength,
			"UsernamePattern":   model.UsernamePattern,
			"Errors":            validationErrs,
		}, "layout")
	}

	if err := u.repository.Update(user); err != nil {
		return fiber.ErrInternalServerError
	}

	if session.Uuid == user.Uuid {
		err = auth.PersistAsCookie(c, user, u.config.SessionTimeout, u.config.Secret)
		if err != nil {
			return fiber.ErrInternalServerError
		}
		c.Locals("Session", user)
	}

	return c.Render("users/edit", fiber.Map{
		"Title":             "Edit user",
		"User":              user,
		"MinPasswordLength": u.config.MinPasswordLength,
		"UsernamePattern":   model.UsernamePattern,
		"Errors":            validationErrs,
		"Message":           "Profile updated",
	}, "layout")
}

func (u *Controller) validate(c *fiber.Ctx, user *model.User, session model.User) (map[string]string, error) {
	errs := user.Validate(u.config.MinPasswordLength)

	exists, err := u.usernameExists(c, session)
	if err != nil {
		log.Println(err.Error())
		return nil, fiber.ErrInternalServerError
	}

	if exists {
		errs["username"] = "A user with this username already exists"
	}

	exists, err = u.emailExists(c, session)
	if err != nil {
		log.Println(err.Error())
		return nil, fiber.ErrInternalServerError
	}

	if exists {
		errs["email"] = "A user with this email address already exists"
	}
	return errs, nil
}

func (u *Controller) usernameExists(c *fiber.Ctx, session model.User) (bool, error) {
	user, err := u.repository.FindByUsername(c.FormValue("username"))
	if err != nil {
		return true, fiber.ErrInternalServerError
	}
	if user != nil && (session.Role == model.RoleAdmin && user.Uuid == c.FormValue("id")) {
		return false, nil
	}
	if user != nil && (session.Uuid != user.Uuid) {
		return true, nil
	}
	return false, nil
}

func (u *Controller) emailExists(c *fiber.Ctx, session model.User) (bool, error) {
	user, err := u.repository.FindByEmail(c.FormValue("email"))
	if err != nil {
		return true, fiber.ErrInternalServerError
	}
	if user != nil && (session.Role == model.RoleAdmin && user.Uuid == c.FormValue("id")) {
		return false, nil
	}
	if user != nil && session.Uuid != user.Uuid {
		return true, nil
	}
	return false, nil
}

// updateUserPassword gathers information from the edit user form and updates user password
func (u *Controller) updateUserPassword(c *fiber.Ctx, session, user model.User) error {
	user.Password = c.FormValue("password")

	errs := user.Validate(u.config.MinPasswordLength)

	// Allow admins to change password of other users without entering user's current password
	if session.Uuid == c.FormValue("id") {
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
			"MinPasswordLength": u.config.MinPasswordLength,
			"ActiveTab":         "password",
			"UsernamePattern":   model.UsernamePattern,
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
		"MinPasswordLength": u.config.MinPasswordLength,
		"ActiveTab":         "password",
		"UsernamePattern":   model.UsernamePattern,
		"Errors":            errs,
		"Message":           "Password updated",
	}, "layout")
}
