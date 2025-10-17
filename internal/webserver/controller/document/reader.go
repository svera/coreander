package document

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func (d *Controller) Reader(c *fiber.Ctx) error {
	document, err := d.idx.Document(c.Params("slug"))
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	if document.Slug == "" {
		return fiber.ErrNotFound
	}

	if _, err := os.Stat(filepath.Join(d.config.LibraryPath, document.ID)); err != nil {
		return fiber.ErrNotFound
	}

	title := document.Title
	authors := strings.Join(document.Authors, ", ")
	if authors != "" {
		title = fmt.Sprintf("%s - %s", authors, document.Title)
	}
	return c.Render("document/reader", fiber.Map{
		"Title":       title,
		"Author":      strings.Join(document.Authors, ", "),
		"Description": document.Description,
		"Slug":        document.Slug,
	})
}
