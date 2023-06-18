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

	if _, err := os.Stat(fmt.Sprintf("%s%s%s", libraryPath, string(os.PathSeparator), document.ID)); err != nil {
		return fiber.ErrNotFound
	}

	template := "epub-reader"
	if strings.ToLower(filepath.Ext(document.ID)) == ".pdf" {
		template = "pdf-reader"
	}

	return c.Render(template, fiber.Map{
		"Lang":  lang,
		"Title": fmt.Sprintf("%s - %s | Coreander", strings.Join(document.Authors, ", "), document.Title),
		"Slug":  document.Slug,
	})

}
