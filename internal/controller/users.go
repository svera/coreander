package controller

import (
	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/internal/model"
)

type Users struct {
	repository *model.Users
}

func NewUsers(repository *model.Users) *Users {
	return &Users{
		repository: repository,
	}
}

func (u *Users) List(c *fiber.Ctx) error {
	users, _ := u.repository.List()
	return c.Render("users/index", fiber.Map{
		"Lang":  c.Params("lang"),
		"Title": "Users",
		"Users": users,
	}, "layout")
}
