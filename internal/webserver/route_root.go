package webserver

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
)

func routeRoot(c *fiber.Ctx) error {
	baseLang := getBaseLanguage(c)
	return c.Redirect(fmt.Sprintf("/%s", baseLang))
}
