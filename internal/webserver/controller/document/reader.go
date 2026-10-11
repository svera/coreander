package document

import (
	"log"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v5/internal/webserver/model"
)

func (d *Controller) Reader(c fiber.Ctx) error {
	document, err := d.idx.Document(c.Params("slug"))
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	if document.Slug == "" {
		return fiber.ErrNotFound
	}

	// Touch the reading record to track that the document has been opened
	// This creates a record if it doesn't exist, but doesn't overwrite existing positions
	var session model.Session
	if val, ok := c.Locals("Session").(model.Session); ok {
		session = val
	}
	if session.ID > 0 {
		if err := d.readingRepository.Touch(int(session.ID), document.Slug); err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
	}

	title, authors := documentPageTitle(document)
	return c.Render("document/reader", fiber.Map{
		"Title":               title,
		"Author":              authors,
		"Description":         document.Description,
		"Slug":                document.Slug,
		"SupportsAnnotations": document.SupportsAnnotations(),
	})
}
