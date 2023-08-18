package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func DocReader(c *fiber.Ctx, libraryPath string, idx IdxReader) error {
	lang := c.Params("lang")

	document, err := idx.Document(c.Params("slug"))
	if err != nil {
		fmt.Println(err)
		return fiber.ErrBadRequest
	}

	if _, err := os.Stat(filepath.Join(libraryPath, document.ID)); err != nil {
		return fiber.ErrNotFound
	}

	template := "epub-reader"
	if strings.ToLower(filepath.Ext(document.ID)) == ".pdf" {
		template = "pdf-reader"
	}

	title := fmt.Sprintf("%s | Coreander", document.Title)
	authors := strings.Join(document.Authors, ", ")
	if authors != "" {
		title = fmt.Sprintf("%s - %s | Coreander", authors, document.Title)
	}
	return c.Render(template, fiber.Map{
		"Lang":        lang,
		"Title":       title,
		"Author":      strings.Join(document.Authors, ", "),
		"Description": document.Description,
		"Slug":        document.Slug,
	})

}
