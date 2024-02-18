package document

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v3/internal/webserver/infrastructure"
	"github.com/svera/coreander/v3/internal/webserver/model"
)

func (d *Controller) Detail(c *fiber.Ctx) error {
	emailSendingConfigured := true
	if _, ok := d.sender.(*infrastructure.NoEmail); ok {
		emailSendingConfigured = false
	}

	var session model.User
	if val, ok := c.Locals("Session").(model.User); ok {
		session = val
	}

	if session.WordsPerMinute > 0 {
		d.config.WordsPerMinute = session.WordsPerMinute
	}

	document, err := d.idx.Document(c.Params("slug"))
	if err != nil {
		return fiber.ErrNotFound
	}

	if _, err := os.Stat(filepath.Join(d.config.LibraryPath, document.ID)); err != nil {
		return fiber.ErrNotFound
	}

	title := fmt.Sprintf("%s | Coreander", document.Title)
	authors := strings.Join(document.Authors, ", ")
	if authors != "" {
		title = fmt.Sprintf("%s - %s | Coreander", authors, document.Title)
	}

	sameSubjects, err := d.idx.SameSubjects(document.Slug, relatedDocuments)
	if err != nil {
		fmt.Println(err)
	}

	sameAuthors, err := d.idx.SameAuthors(document.Slug, relatedDocuments)
	if err != nil {
		fmt.Println(err)
	}

	sameSeries, err := d.idx.SameSeries(document.Slug, relatedDocuments)
	if err != nil {
		fmt.Println(err)
	}

	if session.ID > 0 {
		document = d.hlRepository.Highlighted(int(session.ID), document)
	}

	return c.Render("document", fiber.Map{
		"Title":                  title,
		"Document":               document,
		"EmailSendingConfigured": emailSendingConfigured,
		"EmailFrom":              d.sender.From(),
		"SameSeries":             sameSeries,
		"SameAuthors":            sameAuthors,
		"SameSubjects":           sameSubjects,
		"WordsPerMinute":         d.config.WordsPerMinute,
	}, "layout")
}
