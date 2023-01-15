package webserver

import (
	"fmt"
	"net/url"
	"os"

	"github.com/gofiber/fiber/v2"
)

func routeReader(c *fiber.Ctx, libraryPath string) error {
	lang := c.Params("lang")
	if lang != "es" && lang != "en" {
		return fiber.ErrNotFound
	}

	encodedFilename := c.Params("filename")
	filename, err := url.QueryUnescape(encodedFilename)
	if err != nil {
		return fiber.ErrInternalServerError
	}

	if _, err := os.Stat(fmt.Sprintf("%s/%s", libraryPath, filename)); err != nil {
		return fiber.ErrNotFound
	}

	return c.Render("epub-reader", fiber.Map{
		"Lang":     lang,
		"Title":    "Coreander",
		"Filename": filename,
	})

}
