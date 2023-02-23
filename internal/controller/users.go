package controller

import (
	"fmt"
	"math"
	"net/mail"
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
	Exist(email string) bool
	Admins() int64
	Delete(uuid string) error
	CheckCredentials(email, password string) (model.User, error)
}

type Users struct {
	repository        usersRepository
	minPasswordLength int
}

type newUserFormData struct {
	Name            string `form:"name"`
	Email           string `form:"email"`
	SendToEmail     string `form:"send-to-email"`
	Password        string `form:"password"`
	ConfirmPassword string `form:"confirm-password"`
	Role            int    `form:"role"`
}

type editUserFormData struct {
	Name        string `form:"name"`
	SendToEmail string `form:"send-to-email"`
	Role        int    `form:"role"`
}

type updatePasswordFormData struct {
	OldPassword     string `form:"old-password"`
	Password        string `form:"password"`
	ConfirmPassword string `form:"confirm-password"`
}

type deleteUserFormData struct {
	Uuid string `form:"uuid"`
}

// NewUsers returns a new instance of the users controller
func NewUsers(repository usersRepository, minPasswordLength int) *Users {
	return &Users{
		repository:        repository,
		minPasswordLength: minPasswordLength,
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
	totalPages := int(math.Ceil(float64(totalRows) / float64(model.ResultsPerPage)))

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
	}, "layout")
}

// New renders the new user form
func (u *Users) New(c *fiber.Ctx) error {
	session := jwtclaimsreader.SessionData(c)

	return c.Render("users/new", fiber.Map{
		"Lang":              c.Params("lang"),
		"Title":             "Add new user",
		"Session":           session,
		"Version":           c.App().Config().AppName,
		"MinPasswordLength": u.minPasswordLength,
	}, "layout")
}

// Create gathers information coming from the new user form and creates a new user
func (u *Users) Create(c *fiber.Ctx) error {
	data := new(newUserFormData)
	session := jwtclaimsreader.SessionData(c)

	if err := c.BodyParser(data); err != nil {
		return err
	}

	if errs := u.validateNew(data); len(errs) > 0 {
		return c.Render("users/new", fiber.Map{
			"Lang":    c.Params("lang"),
			"Title":   "Add new user",
			"Session": session,
			"Version": c.App().Config().AppName,
			"Errors":  errs,
		}, "layout")
	}

	user := model.User{
		Name:     data.Name,
		Email:    data.Email,
		Password: model.Hash(data.Password),
		Role:     data.Role,
		Uuid:     uuid.NewString(),
	}

	if err := u.repository.Create(user); err != nil {
		return c.Render("users/new", fiber.Map{
			"Lang":     c.Params("lang"),
			"Title":    "Add new user",
			"UserData": session,
			"Version":  c.App().Config().AppName,
		}, "layout")
	}

	return c.Redirect(fmt.Sprintf("/%s/users", c.Params("lang")))
}

// Update gathers information from the edit user form and updates user data
func (u *Users) Update(c *fiber.Ctx) error {
	var (
		err  error
		user model.User
	)

	data := new(editUserFormData)
	session := jwtclaimsreader.SessionData(c)

	if err = c.BodyParser(data); err != nil {
		return err
	}

	if session.Role != model.RoleAdmin && session.Uuid != c.Params("uuid") {
		if session.Role != model.RoleAdmin {
			return fiber.ErrForbidden
		}
	}

	if user, err = u.repository.Find(c.Params("uuid")); err != nil {
		return fiber.ErrNotFound
	}
	user.Name = data.Name
	user.SendToEmail = data.SendToEmail

	if errs := u.validateUpdate(data); len(errs) > 0 {
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

// Update gathers information from the edit user form and updates user password
func (u *Users) UpdatePassword(c *fiber.Ctx) error {
	var (
		err  error
		user model.User
	)

	data := new(updatePasswordFormData)
	session := jwtclaimsreader.SessionData(c)

	if err := c.BodyParser(data); err != nil {
		return err
	}

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
	user.Password = model.Hash(data.Password)

	errs := []string{}

	// Allow admins to change password of other users without entering user's current password
	if session.Uuid == c.Params("uuid") {
		if _, err := u.repository.CheckCredentials(user.Email, data.OldPassword); err != nil {
			errs = append(errs, "The current password is not correct")
		}
	}
	if errs = u.validatePassword(data.Password, data.ConfirmPassword, errs); len(errs) > 0 {
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

	data := new(deleteUserFormData)

	if err := c.BodyParser(data); err != nil {
		return err
	}

	u.repository.Delete(data.Uuid)
	return c.Redirect(fmt.Sprintf("/%s/users", c.Params("lang")))
}

func (u *Users) validateNew(data *newUserFormData) []string {
	errs := []string{}

	if data.Name == "" {
		errs = append(errs, "Name cannot be empty")
	}

	if _, err := mail.ParseAddress(data.Email); err != nil {
		errs = append(errs, "Incorrect email address")
	}

	if u.repository.Exist(data.Email) {
		errs = append(errs, "A user with that email address already exist")
	}

	if _, err := mail.ParseAddress(data.SendToEmail); data.SendToEmail != "" && err != nil {
		errs = append(errs, "Incorrect send to email address")
	}

	if data.Role < 1 || data.Role > 2 {
		errs = append(errs, "Incorrect role")
	}

	return u.validatePassword(data.Password, data.ConfirmPassword, errs)
}

func (u *Users) validateUpdate(data *editUserFormData) []string {
	errs := []string{}

	if data.Name == "" {
		errs = append(errs, "Name cannot be empty")
	}

	if _, err := mail.ParseAddress(data.SendToEmail); data.SendToEmail != "" && err != nil {
		errs = append(errs, "Incorrect send to email address")
	}

	return errs
}

func (u *Users) validatePassword(password string, confirmPassword string, errs []string) []string {
	if len(password) < u.minPasswordLength {
		errs = append(errs, fmt.Sprintf("Password must be longer than %d characters", u.minPasswordLength))
	}

	if confirmPassword == "" {
		errs = append(errs, "Confirm password cannot be empty")
	}

	if password != confirmPassword {
		errs = append(errs, "Password and confirmation do not match")
	}

	return errs
}
