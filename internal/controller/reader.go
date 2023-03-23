package controller

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
)

func DocReader(c *fiber.Ctx, libraryPath string) error {
	lang := c.Params("lang")

	encodedFilename := c.Params("filename")
	filename, err := url.QueryUnescape(encodedFilename)
	if err != nil {
		return fiber.ErrInternalServerError
	}

	if _, err := os.Stat(fmt.Sprintf("%s/%s", libraryPath, filename)); err != nil {
		return fiber.ErrNotFound
	}

	if filepath.Ext(filename) == ".pdf" {
		return c.Redirect(fmt.Sprintf("/files/%s", encodedFilename))
	}

	return c.Render("epub-reader", fiber.Map{
		"Lang":     lang,
		"Title":    "Coreander",
		"Filename": filename,
	})

}
