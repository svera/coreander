package document

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

// CompletionDate returns just the completion date dd element for a document (used for HTMX updates)
func (d *Controller) CompletionDate(c *fiber.Ctx) error {
	var session model.Session
	if val, ok := c.Locals("Session").(model.Session); ok {
		session = val
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

	return c.Render("partials/completion-date", fiber.Map{
		"Document": document,
	})
}

