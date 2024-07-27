package auth

import (
	"github.com/gofiber/fiber/v2"
)

// Logs out user and removes their JWT.
func (a *Controller) SignOut(c *fiber.Ctx) error {
	c.Cookie(&fiber.Cookie{
		Name:     "coreander",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   false,
		HTTPOnly: true,
	})
	c.Set("HX-Refresh", "true")
	return c.SendStatus(fiber.StatusNoContent)
}
