package auth

import (
	"github.com/gofiber/fiber/v2"
)

func (a *Controller) Login(c *fiber.Ctx) error {
	return c.Render("auth/login", fiber.Map{
		"Title":            a.translator.T(c.Locals("Lang").(string), "Sign in"),
		"DisableLoginLink": true,
	}, "layout")
}
