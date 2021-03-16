package webserver

import (
	"github.com/gofiber/fiber/v2"
)

func routeLogInForm(c *fiber.Ctx, version string) error {
	lang := c.Params("lang")

	if lang != "es" && lang != "en" {
		return fiber.ErrNotFound
	}

	return c.Render("login", fiber.Map{
		"Lang":    lang,
		"Title":   "Coreander",
		"Version": version,
	}, "layout")
}
