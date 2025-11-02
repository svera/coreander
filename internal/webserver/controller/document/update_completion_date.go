package document

import (
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

type updateCompletionDateRequest struct {
	CompletedAt string `json:"completed_at"`
}

// UpdateCompletionDate updates the completion date for a document
func (d *Controller) UpdateCompletionDate(c *fiber.Ctx) error {
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

	var req updateCompletionDateRequest
	if err := c.BodyParser(&req); err != nil {
		log.Printf("error parsing request body: %s\n", err)
		return fiber.ErrBadRequest
	}

	// Parse the date string (expecting format: YYYY-MM-DD)
	completedAt, err := time.Parse("2006-01-02", req.CompletedAt)
	if err != nil {
		log.Printf("error parsing date: %s\n", err)
		return fiber.ErrBadRequest
	}

	// Update the completion date
	if err := d.readingRepository.UpdateCompletionDate(int(session.ID), document.ID, completedAt); err != nil {
		log.Printf("error updating completion date: %s\n", err)
		return fiber.ErrInternalServerError
	}

	// Return 204 No Content for successful operation
	return c.SendStatus(fiber.StatusNoContent)
}

