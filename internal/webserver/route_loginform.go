package webserver

import (
	"github.com/gofiber/fiber/v2"
)

func routeLogInForm(c *fiber.Ctx, version string) error {
	lang := c.Params("lang")

	if lang != "es" && lang != "en" {
		return fiber.ErrNotFound
	}

	forbidden := false
	if c.Query("forbidden") == "1" {
		forbidden = true
	}

	return c.Render("login", fiber.Map{
		"Lang":      lang,
		"Title":     "Coreander",
		"Version":   version,
		"Forbidden": forbidden,
	}, "layout")
}
