package controller

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/svera/coreander/internal/model"
)

type Users struct {
	repository *model.Users
}

type newUserFormData struct {
	Name           string `form:"name"`
	Username       string `form:"username"`
	Password       string `form:"password"`
	RepeatPassword string `form:"repeat-password"`
	Role           int    `form:"role"`
}

func NewUsers(repository *model.Users) *Users {
	return &Users{
		repository: repository,
	}
}

func (u *Users) List(c *fiber.Ctx) error {
	c.Append("Cache-Time", "0")
	user := c.Locals("user").(*jwt.Token)
	claims := user.Claims.(jwt.MapClaims)
	if claims["role"].(float64) != model.RoleAdmin {
		return fiber.ErrForbidden
	}
	fmt.Printf("%v", claims)
	users, _ := u.repository.List()
	return c.Render("users/index", fiber.Map{
		"Lang":  c.Params("lang"),
		"Title": "Users",
		"Users": users,
	}, "layout")
}

func (u *Users) Edit(c *fiber.Ctx) error {
	if c.Params("uuid") != "" {
		return fiber.ErrBadRequest
	}
	user, _ := u.repository.Find(c.Params("uuid"))
	return c.Render("users/edit", fiber.Map{
		"Lang":  c.Params("lang"),
		"Title": "Users",
		"User":  user,
	}, "layout")
}

func (u *Users) New(c *fiber.Ctx) error {
	return c.Render("users/new", fiber.Map{
		"Lang":  c.Params("lang"),
		"Title": "Add new user",
	}, "layout")
}

func (u *Users) Create(c *fiber.Ctx) error {
	data := new(newUserFormData)

	if err := c.BodyParser(data); err != nil {
		return err
	}

	if !validate(data) {
		return c.Render("users/new", fiber.Map{
			"Lang":  c.Params("lang"),
			"Title": "Add new user",
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
			"Lang":  c.Params("lang"),
			"Title": "Add new user",
		}, "layout")
	}

	return c.Redirect(fmt.Sprintf("/%s/users", c.Params("lang")))
}

func validate(userFormData *newUserFormData) bool {
	if userFormData.Password != userFormData.RepeatPassword {
		return false
	}
	return true
}
