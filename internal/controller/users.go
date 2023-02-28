package controller

import (
	"fmt"
	"math"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/svera/coreander/internal/jwtclaimsreader"
	"github.com/svera/coreander/internal/model"
)

type usersRepository interface {
	List(page int, resultsPerPage int) ([]model.User, error)
	Total() int64
	Find(uuid string) (model.User, error)
	Create(user model.User) error
	Update(user model.User) error
	FindByEmail(email string) model.User
	Admins() int64
	Delete(uuid string) error
	CheckCredentials(email, password string) (model.User, error)
}

type Users struct {
	repository        usersRepository
	minPasswordLength int
	wordsPerMinute    float64
}

// NewUsers returns a new instance of the users controller
func NewUsers(repository usersRepository, minPasswordLength int, wordsPerMinute float64) *Users {
	return &Users{
		repository:        repository,
		minPasswordLength: minPasswordLength,
		wordsPerMinute:    wordsPerMinute,
	}
}

// List list all users registered in the database
func (u *Users) List(c *fiber.Ctx) error {
	session := jwtclaimsreader.SessionData(c)

	if session.Role != model.RoleAdmin {
		return fiber.ErrForbidden
	}

	page, err := strconv.Atoi(c.Query("page"))
	if err != nil {
		page = 1
	}
	totalRows := u.repository.Total()
	totalPages := int(math.Ceil(float64(totalRows) / model.ResultsPerPage))

	users, _ := u.repository.List(page, model.ResultsPerPage)
	return c.Render("users/index", fiber.Map{
		"Lang":      c.Params("lang"),
		"Title":     "Users",
		"Users":     users,
		"Paginator": pagination(model.MaxPagesNavigator, totalPages, page, map[string]string{}),
		"Session":   session,
		"Version":   c.App().Config().AppName,
		"Admins":    u.repository.Admins(),
	}, "layout")
}

// Edit renders the edit user form
func (u *Users) Edit(c *fiber.Ctx) error {
	if c.Params("uuid") == "" {
		return fiber.ErrBadRequest
	}

	session := jwtclaimsreader.SessionData(c)
	if session.Role != model.RoleAdmin && session.Uuid != c.Params("uuid") {
		if session.Role != model.RoleAdmin {
			return fiber.ErrForbidden
		}
	}

	user, _ := u.repository.Find(c.Params("uuid"))
	return c.Render("users/edit", fiber.Map{
		"Lang":    c.Params("lang"),
		"Title":   "Edit user",
		"User":    user,
		"Session": session,
		"Version": c.App().Config().AppName,
		"Errors":  map[string]string{},
	}, "layout")
}

// New renders the new user form
func (u *Users) New(c *fiber.Ctx) error {
	session := jwtclaimsreader.SessionData(c)

	if session.Role != model.RoleAdmin {
		return fiber.ErrForbidden
	}

	user := model.User{
		WordsPerMinute: u.wordsPerMinute,
	}
	return c.Render("users/new", fiber.Map{
		"Lang":              c.Params("lang"),
		"Title":             "Add new user",
		"Session":           session,
		"Version":           c.App().Config().AppName,
		"MinPasswordLength": u.minPasswordLength,
		"User":              user,
		"Errors":            map[string]string{},
	}, "layout")
}

// Create gathers information coming from the new user form and creates a new user
func (u *Users) Create(c *fiber.Ctx) error {
	session := jwtclaimsreader.SessionData(c)

	if session.Role != model.RoleAdmin {
		return fiber.ErrForbidden
	}

	role, _ := strconv.Atoi(c.FormValue("role"))
	user := model.User{
		Name:     c.FormValue("name"),
		Email:    c.FormValue("email"),
		Password: model.Hash(c.FormValue("password")),
		Role:     role,
		Uuid:     uuid.NewString(),
	}
	user.WordsPerMinute, _ = strconv.ParseFloat(c.FormValue("words-per-minute"), 64)

	errs := user.Validate(u.minPasswordLength)
	if exist := u.repository.FindByEmail(c.FormValue("email")); exist.Email != "" {
		errs["email"] = "A user with this email address already exist"
	}
	errs = u.confirmPassword(c.FormValue("password"), c.FormValue("confirm-password"), errs)

	if len(errs) > 0 {
		return c.Render("users/new", fiber.Map{
			"Lang":    c.Params("lang"),
			"Title":   "Add new user",
			"Session": session,
			"Version": c.App().Config().AppName,
			"Errors":  errs,
			"User":    user,
		}, "layout")
	}

	if err := u.repository.Create(user); err != nil {
		return fiber.ErrInternalServerError
	}

	return c.Redirect(fmt.Sprintf("/%s/users", c.Params("lang")))
}

// Update gathers information from the edit user form and updates user data
func (u *Users) Update(c *fiber.Ctx) error {
	var (
		err  error
		user model.User
	)

	session := jwtclaimsreader.SessionData(c)

	if session.Role != model.RoleAdmin && session.Uuid != c.Params("uuid") {
		if session.Role != model.RoleAdmin {
			return fiber.ErrForbidden
		}
	}

	if user, err = u.repository.Find(c.Params("uuid")); err != nil {
		return fiber.ErrNotFound
	}
	user.Name = c.FormValue("name")
	user.SendToEmail = c.FormValue("send-to-email")
	user.WordsPerMinute, _ = strconv.ParseFloat(c.FormValue("words-per-minute"), 64)

	errs := user.Validate(u.minPasswordLength)
	if len(errs) > 0 {
		return c.Render("users/edit", fiber.Map{
			"Lang":    c.Params("lang"),
			"Title":   "Edit user",
			"User":    user,
			"Session": session,
			"Version": c.App().Config().AppName,
			"Errors":  errs,
		}, "layout")
	}

	if err := u.repository.Update(user); err != nil {
		return fiber.ErrInternalServerError
	}

	return c.Render("users/edit", fiber.Map{
		"Lang":    c.Params("lang"),
		"Title":   "Edit user",
		"User":    user,
		"Session": session,
		"Version": c.App().Config().AppName,
		"Message": "Profile updated",
	}, "layout")
}

// UpdatePassword gathers information from the edit user form and updates user password
func (u *Users) UpdatePassword(c *fiber.Ctx) error {
	var (
		err  error
		user model.User
	)

	session := jwtclaimsreader.SessionData(c)

	if session.Role != model.RoleAdmin && session.Uuid != c.Params("uuid") {
		return c.Status(fiber.StatusForbidden).Render(
			"errors/forbidden",
			fiber.Map{
				"Lang":    c.Params("lang"),
				"Title":   "Forbidden",
				"Session": session,
				"Version": c.App().Config().AppName,
			},
			"layout",
		)
	}

	if user, err = u.repository.Find(c.Params("uuid")); err != nil {
		return fiber.ErrNotFound
	}
	user.Password = model.Hash(c.FormValue("password"))

	errs := user.Validate(u.minPasswordLength)

	// Allow admins to change password of other users without entering user's current password
	if session.Uuid == c.Params("uuid") {
		if _, err := u.repository.CheckCredentials(user.Email, c.FormValue("old-password")); err != nil {
			errs["oldpassword"] = "The current password is not correct"
		}
	}
	errs = u.confirmPassword(c.FormValue("password"), c.FormValue("confirm-password"), errs)
	if len(errs) > 0 {
		return c.Render("users/edit", fiber.Map{
			"Lang":      c.Params("lang"),
			"Title":     "Edit user",
			"User":      user,
			"Session":   session,
			"Version":   c.App().Config().AppName,
			"ActiveTab": "password",
			"Errors":    errs,
		}, "layout")
	}

	if err := u.repository.Update(user); err != nil {
		return fiber.ErrInternalServerError
	}

	return c.Render("users/edit", fiber.Map{
		"Lang":      c.Params("lang"),
		"Title":     "Edit user",
		"User":      user,
		"Session":   session,
		"Version":   c.App().Config().AppName,
		"ActiveTab": "password",
		"Message":   "Password updated",
	}, "layout")
}

// Delete soft-removes a user from the database
func (u *Users) Delete(c *fiber.Ctx) error {
	session := jwtclaimsreader.SessionData(c)

	if session.Role != model.RoleAdmin && session.Uuid != c.Params("uuid") {
		if session.Role != model.RoleAdmin {
			return fiber.ErrForbidden
		}
	}

	user, err := u.repository.Find(c.FormValue("uuid"))
	if err != nil {
		return fiber.ErrNotFound
	}
	if u.repository.Admins() == 1 && user.Role == model.RoleAdmin {
		return fiber.ErrForbidden
	}

	u.repository.Delete(c.FormValue("uuid"))
	return c.Redirect(fmt.Sprintf("/%s/users", c.Params("lang")))
}

func (u *Users) confirmPassword(password string, confirmPassword string, errs map[string]string) map[string]string {
	if confirmPassword == "" {
		errs["confirmpassword"] = "Confirm password cannot be empty"
	}

	if password != confirmPassword {
		errs["confirmpassword"] = "Password and confirmation do not match"
	}

	return errs
}
