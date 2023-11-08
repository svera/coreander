package auth

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Logs out user and removes their JWT.
func (a *Controller) SignOut(c *fiber.Ctx) error {
	c.Cookie(&fiber.Cookie{
		Name:     "coreander",
		Value:    "",
		Path:     "/",
		Expires:  time.Now().Add(-time.Second * 10),
		Secure:   false,
		HTTPOnly: true,
	})

	return c.Redirect(fmt.Sprintf("/%s", c.Params("lang")))
}
