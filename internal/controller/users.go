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

type Users struct {
	repository *model.Users
	version    string
}

type newUserFormData struct {
	Name           string  `form:"name"`
	Username       string  `form:"username"`
	Password       string  `form:"password"`
	RepeatPassword string  `form:"repeat-password"`
	Role           float64 `form:"role"`
}

type deleteUserFormData struct {
	Uuid string `form:"uuid"`
}

func NewUsers(repository *model.Users, version string) *Users {
	return &Users{
		repository: repository,
		version:    version,
	}
}

func (u *Users) List(c *fiber.Ctx) error {
	userData := jwtclaimsreader.UserData(c)

	if userData.Role != model.RoleAdmin {
		return c.Status(fiber.StatusForbidden).Render(
			"errors/forbidden",
			fiber.Map{
				"Lang":     c.Params("lang"),
				"Title":    "Forbidden",
				"UserData": userData,
				"Version":  u.version,
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
		"UserData":  userData,
		"Version":   u.version,
		"Admins":    u.repository.Admins(),
	}, "layout")
}

func (u *Users) Edit(c *fiber.Ctx) error {
	if c.Params("uuid") == "" {
		return fiber.ErrBadRequest
	}

	userData := jwtclaimsreader.UserData(c)
	if userData.Role != model.RoleAdmin && userData.Uuid != c.Params("uuid") {
		return c.Status(fiber.StatusForbidden).Render(
			"errors/forbidden",
			fiber.Map{
				"Lang":     c.Params("lang"),
				"Title":    "Forbidden",
				"UserData": userData,
				"Version":  u.version,
			},
			"layout",
		)
	}

	user, _ := u.repository.Find(c.Params("uuid"))
	return c.Render("users/edit", fiber.Map{
		"Lang":     c.Params("lang"),
		"Title":    "Users",
		"User":     user,
		"UserData": userData,
		"Version":  u.version,
	}, "layout")
}

func (u *Users) New(c *fiber.Ctx) error {
	userData := jwtclaimsreader.UserData(c)

	return c.Render("users/new", fiber.Map{
		"Lang":     c.Params("lang"),
		"Title":    "Add new user",
		"UserData": userData,
		"Version":  u.version,
	}, "layout")
}

func (u *Users) Create(c *fiber.Ctx) error {
	data := new(newUserFormData)
	userData := jwtclaimsreader.UserData(c)

	if err := c.BodyParser(data); err != nil {
		return err
	}

	if errs := u.validate(data); len(errs) > 0 {
		return c.Render("users/new", fiber.Map{
			"Lang":     c.Params("lang"),
			"Title":    "Add new user",
			"UserData": userData,
			"Version":  u.version,
			"Errors":   errs,
		}, "layout")
	}

	user := model.User{
		Name:     data.Name,
		Username: data.Username,
		Password: model.Hash(data.Password),
		Role:     data.Role,
		Uuid:     uuid.NewString(),
	}

	if err := u.repository.Create(user); err != nil {
		return c.Render("users/new", fiber.Map{
			"Lang":     c.Params("lang"),
			"Title":    "Add new user",
			"UserData": userData,
			"Version":  u.version,
		}, "layout")
	}

	return c.Redirect(fmt.Sprintf("/%s/users", c.Params("lang")))
}

func (u *Users) Delete(c *fiber.Ctx) error {
	userData := jwtclaimsreader.UserData(c)
	if userData.Role != model.RoleAdmin && userData.Uuid != c.Params("uuid") {
		return c.Status(fiber.StatusForbidden).Render(
			"errors/forbidden",
			fiber.Map{
				"Lang":     c.Params("lang"),
				"Title":    "Forbidden",
				"UserData": userData,
				"Version":  u.version,
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

func (u *Users) validate(userFormData *newUserFormData) []string {
	errs := []string{}
	if userFormData.Name == "" {
		errs = append(errs, "Name cannot be empty")
	}
	if userFormData.Role < 1 || userFormData.Role > 2 {
		errs = append(errs, "Incorrect role")
	}
	if userFormData.Password == "" {
		errs = append(errs, "Password cannot be empty")
	}
	if userFormData.RepeatPassword == "" {
		errs = append(errs, "Repeat password cannot be empty")
	}
	if userFormData.Password != userFormData.RepeatPassword {
		errs = append(errs, "Password and confirmation do not match")
	}
	if u.repository.Exist(userFormData.Username) {
		errs = append(errs, "Username already exist")
	}
	return errs
}
