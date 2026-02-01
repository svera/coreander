package user

import (
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/webserver/controller/auth"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

// Update gathers information from the edit user form and updates user data
func (u *Controller) Update(c *fiber.Ctx) error {
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

	if session.Role != model.RoleAdmin && user.Username != session.Username {
		return fiber.ErrForbidden
	}

	var validationErrs map[string]string

	switch c.FormValue("tab") {
	case "profile":
		validationErrs, err = u.updateUserData(c, user, session)
	case "password":
		validationErrs, err = u.updateUserPassword(c, *user, session)
	default:
		err = u.updateOptions(c, user, session)
	}

	if err != nil {
		log.Println(err.Error())
		return fiber.ErrInternalServerError
	}

	// Calculate yearly reading statistics
	yearlyCompletedCount, yearlyReadingTime := u.calculateYearlyStats(int(user.ID), user.WordsPerMinute)

	// Calculate lifetime reading statistics
	lifetimeCompletedCount, lifetimeReadingTime := u.calculateLifetimeStats(int(user.ID), user.WordsPerMinute)

	vars := fiber.Map{
		"Title":                  "Edit user",
		"User":                   user,
		"MinPasswordLength":      u.config.MinPasswordLength,
		"UsernamePattern":        model.UsernamePattern,
		"Errors":                 validationErrs,
		"EmailFrom":              u.sender.From(),
		"ActiveTab":              c.FormValue("tab"),
		"YearlyCompletedCount":   yearlyCompletedCount,
		"YearlyReadingTime":      yearlyReadingTime,
		"LifetimeCompletedCount": lifetimeCompletedCount,
		"LifetimeReadingTime":    lifetimeReadingTime,
	}

	if len(validationErrs) > 0 {
		c.Status(fiber.StatusBadRequest)
	}

	return c.Render("user/edit", vars)
}

func (u *Controller) updateOptions(c *fiber.Ctx, user *model.User, session model.Session) error {
	user.ShowFileName = c.FormValue("show-file-name") == "on"
	if c.FormValue("private-profile") == "on" {
		user.PrivateProfile = 1
	} else {
		user.PrivateProfile = 0
	}
	user.SendToEmail = c.FormValue("send-to-email")
	user.PreferredEpubType = strings.ToLower(c.FormValue("preferred-epub-type"))
	user.DefaultAction = strings.ToLower(c.FormValue("default-action"))

	if user.PreferredEpubType != "epub" && user.PreferredEpubType != "kepub" {
		return fiber.ErrBadRequest
	}
	if user.DefaultAction != "" && user.DefaultAction != "download" && user.DefaultAction != "send" && user.DefaultAction != "share" && user.DefaultAction != "copy" {
		return fiber.ErrBadRequest
	}

	// Validate that share and send actions are only allowed if email sending is configured
	if user.DefaultAction == "share" || user.DefaultAction == "send" {
		if _, ok := u.sender.(*infrastructure.NoEmail); ok {
			return fiber.ErrBadRequest
		}
	}

	user.WordsPerMinute, _ = strconv.ParseFloat(c.FormValue("words-per-minute"), 64)

	if err := u.usersRepository.Update(user); err != nil {
		return fiber.ErrInternalServerError
	}

	if err := u.refreshSession(session, user, c); err != nil {
		return fiber.ErrInternalServerError
	}

	return nil
}

func (u *Controller) refreshSession(session model.Session, user *model.User, c *fiber.Ctx) error {
	if session.Uuid == user.Uuid {
		expiration := time.Unix(int64(session.Exp), 0)
		signedToken, err := auth.GenerateToken(c, user, expiration, u.config.Secret)
		if err != nil {
			return err
		}

		c.Cookie(&fiber.Cookie{
			Name:     "session",
			Value:    signedToken,
			Path:     "/",
			MaxAge:   int(session.Exp),
			Secure:   false,
			HTTPOnly: true,
		})
		c.Locals("Session", user)
	}
	return nil
}

func (u *Controller) updateUserData(c *fiber.Ctx, user *model.User, session model.Session) (map[string]string, error) {
	user.Name = strings.TrimSpace(c.FormValue("name"))
	user.Username = strings.ToLower(c.FormValue("username"))
	user.Email = c.FormValue("email")

	validationErrs, err := u.validate(c, user, session)
	if err != nil || len(validationErrs) > 0 {
		return validationErrs, err
	}

	if err := u.usersRepository.Update(user); err != nil {
		return nil, fiber.ErrInternalServerError
	}

	if err := u.refreshSession(session, user, c); err != nil {
		return nil, fiber.ErrInternalServerError
	}
	return nil, nil
}

func (u *Controller) validate(c *fiber.Ctx, user *model.User, session model.Session) (map[string]string, error) {
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

func (u *Controller) usernameExists(c *fiber.Ctx, session model.Session) (bool, error) {
	user, err := u.usersRepository.FindByUsername(c.FormValue("username"))
	if err != nil {
		return false, err
	}
	if user == nil {
		return false, nil
	}
	if session.Role == model.RoleAdmin && user.Uuid == c.FormValue("id") {
		return false, nil
	}
	if session.Uuid != user.Uuid {
		return true, nil
	}
	return false, nil
}

func (u *Controller) emailExists(c *fiber.Ctx, session model.Session) (bool, error) {
	user, err := u.usersRepository.FindByEmail(c.FormValue("email"))
	if err != nil {
		return false, err
	}
	if user == nil {
		return false, nil
	}
	if session.Role == model.RoleAdmin && user.Uuid == c.FormValue("id") {
		return false, nil
	}
	if session.Uuid != user.Uuid {
		return true, nil
	}
	return false, nil
}

// updateUserPassword gathers information from the edit user form and updates user password
func (u *Controller) updateUserPassword(c *fiber.Ctx, user model.User, session model.Session) (map[string]string, error) {
	user.Password = c.FormValue("password")

	errs := user.Validate(u.config.MinPasswordLength)

	// Allow admins to change password of other users without entering user's current password
	if session.Uuid == c.FormValue("id") {
		user, err := u.usersRepository.FindByEmail(user.Email)
		if err != nil {
			return nil, fiber.ErrInternalServerError
		}

		if user.Password != model.Hash(c.FormValue("old-password")) {
			errs["oldpassword"] = "The current password is not correct"
		}
	}

	if errs = user.ConfirmPassword(c.FormValue("confirm-password"), u.config.MinPasswordLength, errs); len(errs) > 0 {
		return errs, nil
	}

	user.Password = model.Hash(user.Password)
	if err := u.usersRepository.Update(&user); err != nil {
		return errs, fiber.ErrInternalServerError
	}

	return nil, nil
}
