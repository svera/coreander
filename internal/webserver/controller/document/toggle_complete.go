package document

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

// ToggleComplete marks a document as complete or incomplete
func (d *Controller) ToggleComplete(c *fiber.Ctx) error {
	var session model.Session
	if val, ok := c.Locals("Session").(model.Session); ok {
		session = val
	}

	if session.ID == 0 {
		return fiber.ErrUnauthorized
	}

	document, err := d.idx.Document(c.Params("slug"))
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	if document.Slug == "" {
		return fiber.ErrNotFound
	}

	// Get current reading status to check if already completed
	reading, err := d.readingRepository.Get(int(session.ID), document.ID)
	if err != nil {
		// If no reading record exists yet, create one by touching it
		if err := d.readingRepository.Touch(int(session.ID), document.ID); err != nil {
			log.Printf("error creating reading record: %s\n", err)
			return fiber.ErrInternalServerError
		}
		reading.Completed = false
	}

	// Toggle completion status
	if reading.Completed {
		// Already complete, mark as incomplete
		if err := d.readingRepository.MarkIncomplete(int(session.ID), document.ID); err != nil {
			log.Printf("error marking document as incomplete: %s\n", err)
			return fiber.ErrInternalServerError
		}
	} else {
		// Not complete, mark as complete
		if err := d.readingRepository.MarkComplete(int(session.ID), document.ID); err != nil {
			log.Printf("error marking document as complete: %s\n", err)
			return fiber.ErrInternalServerError
		}
	}

	// Return 204 No Content for successful toggle
	return c.SendStatus(fiber.StatusNoContent)
}

