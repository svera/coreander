package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func DocReader(c *fiber.Ctx, libraryPath string, idx Reader) error {
	lang := c.Params("lang")

	document, err := idx.Document(c.Params("slug"))
	if err != nil {
		fmt.Println(err)
		return fiber.ErrBadRequest
	}

	if _, err := os.Stat(fmt.Sprintf("%s"+string(os.PathSeparator)+"%s", libraryPath, document.ID)); err != nil {
		return fiber.ErrNotFound
	}

	if strings.ToLower(filepath.Ext(document.ID)) == ".pdf" {
		return c.Redirect(fmt.Sprintf("/download/%s", document.Slug))
	}

	return c.Render("epub-reader", fiber.Map{
		"Lang":  lang,
		"Title": "Coreander",
		"Slug":  document.Slug,
	})

}
