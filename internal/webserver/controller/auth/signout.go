package auth

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
)

// Logs out user and removes their JWT.
func (a *Controller) SignOut(c *fiber.Ctx) error {
	c.Cookie(&fiber.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   false,
		HTTPOnly: true,
	})

	return c.Redirect(fmt.Sprintf("/%s", c.Params("lang")))
}
