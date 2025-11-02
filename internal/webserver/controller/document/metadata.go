package document

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

// Metadata returns just the metadata section for a document (used for HTMX updates)
func (d *Controller) Metadata(c *fiber.Ctx) error {
	var session model.Session
	if val, ok := c.Locals("Session").(model.Session); ok {
		session = val
	}

	if session.WordsPerMinute > 0 {
		d.config.WordsPerMinute = session.WordsPerMinute
	}

	document, err := d.idx.Document(c.Params("slug"))
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	if document.Slug == "" {
		return fiber.ErrNotFound
	}

	if session.ID > 0 {
		document = d.readingRepository.Completed(int(session.ID), document)
	}

	return c.Render("partials/document-metadata", fiber.Map{
		"Document":       document,
		"Session":        session,
		"WordsPerMinute": d.config.WordsPerMinute,
	})
}

