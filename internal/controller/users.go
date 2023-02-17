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

type Users struct {
	repository *model.Users
	version    string
}

type userFormData struct {
	Name            string  `form:"name"`
	Email           string  `form:"email"`
	Password        string  `form:"password"`
	ConfirmPassword string  `form:"confirm-password"`
	Role            float64 `form:"role"`
}

type deleteUserFormData struct {
	Uuid string `form:"uuid"`
}

// NewUsers returns a new instance of the users controller
func NewUsers(repository *model.Users, version string) *Users {
	return &Users{
		repository: repository,
		version:    version,
	}
}

// List list all users registered in the database
func (u *Users) List(c *fiber.Ctx) error {
	session := jwtclaimsreader.SessionData(c)

	if session.Role != model.RoleAdmin {
		return c.Status(fiber.StatusForbidden).Render(
			"errors/forbidden",
			fiber.Map{
				"Lang":    c.Params("lang"),
				"Title":   "Forbidden",
				"Session": session,
				"Version": u.version,
			},
			"layout",
		)
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
		"Version":   u.version,
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
		return c.Status(fiber.StatusForbidden).Render(
			"errors/forbidden",
			fiber.Map{
				"Lang":    c.Params("lang"),
				"Title":   "Forbidden",
				"Session": session,
				"Version": u.version,
			},
			"layout",
		)
	}

	user, _ := u.repository.Find(c.Params("uuid"))
	return c.Render("users/edit", fiber.Map{
		"Lang":    c.Params("lang"),
		"Title":   "Edit user",
		"User":    user,
		"Session": session,
		"Version": u.version,
	}, "layout")
}

// New renders the new user form
func (u *Users) New(c *fiber.Ctx) error {
	session := jwtclaimsreader.SessionData(c)

	return c.Render("users/new", fiber.Map{
		"Lang":    c.Params("lang"),
		"Title":   "Add new user",
		"Session": session,
		"Version": u.version,
	}, "layout")
}

// Create gathers information coming from the new user form and creates a new user
func (u *Users) Create(c *fiber.Ctx) error {
	data := new(userFormData)
	session := jwtclaimsreader.SessionData(c)

	if err := c.BodyParser(data); err != nil {
		return err
	}

	if errs := u.validate(data); len(errs) > 0 {
		return c.Render("users/new", fiber.Map{
			"Lang":    c.Params("lang"),
			"Title":   "Add new user",
			"Session": session,
			"Version": u.version,
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
			"Version":  u.version,
		}, "layout")
	}

	return c.Redirect(fmt.Sprintf("/%s/users", c.Params("lang")))
}

// Update gathers information from the edit user form and updates user data
func (u *Users) Update(c *fiber.Ctx) error {
	data := new(userFormData)
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
				"Version": u.version,
			},
			"layout",
		)
	}

	user := model.User{
		Name:     data.Name,
		Email:    data.Email,
		Password: model.Hash(data.Password),
	}

	if errs := u.validate(data); len(errs) > 0 {
		return c.Render("users/edit", fiber.Map{
			"Lang":    c.Params("lang"),
			"Title":   "Edit user",
			"User":    user,
			"Session": session,
			"Version": u.version,
			"Errors":  errs,
		}, "layout")
	}

	if err := u.repository.Update(user); err != nil {
		return c.Render("users/edit", fiber.Map{
			"Lang":    c.Params("lang"),
			"Title":   "Edit user",
			"User":    user,
			"Session": session,
			"Version": u.version,
			"Message": "Profile succesfully updated",
		}, "layout")
	}

	return c.Redirect(fmt.Sprintf("/%s/users", c.Params("lang")))
}

// Delete soft-removes a user from the database
func (u *Users) Delete(c *fiber.Ctx) error {
	session := jwtclaimsreader.SessionData(c)

	if session.Role != model.RoleAdmin && session.Uuid != c.Params("uuid") {
		return c.Status(fiber.StatusForbidden).Render(
			"errors/forbidden",
			fiber.Map{
				"Lang":    c.Params("lang"),
				"Title":   "Forbidden",
				"Session": session,
				"Version": u.version,
			},
			"layout",
		)
	}

	data := new(deleteUserFormData)

	if err := c.BodyParser(data); err != nil {
		return err
	}

	u.repository.Delete(data.Uuid)
	return c.Redirect(fmt.Sprintf("/%s/users", c.Params("lang")))
}

func (u *Users) validate(data *userFormData) []string {
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
	if data.Role < 1 || data.Role > 2 {
		errs = append(errs, "Incorrect role")
	}
	if data.Password == "" {
		errs = append(errs, "Password cannot be empty")
	}
	if data.ConfirmPassword == "" {
		errs = append(errs, "Confirm password cannot be empty")
	}
	if data.Password != data.ConfirmPassword {
		errs = append(errs, "Password and confirmation do not match")
	}
	return errs
}
