package document

import (
	"log"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

type updateCompletionDateRequest struct {
	CompletedOn *string `json:"completed_on"`
}

// ToggleComplete marks a document as complete or incomplete
// If a date is provided in the request body, it sets the completion date to that value
// If no date is provided (POST), it toggles between complete (with current date) and incomplete
func (d *Controller) ToggleComplete(c fiber.Ctx) error {
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

	// Check if a date was provided in the request body (for updating completion date)
	var req updateCompletionDateRequest
	if c.Body() != nil && len(c.Body()) > 0 {
		if err := c.Bind().Body(&req); err != nil {
			return fiber.ErrBadRequest
		}

		// If completed_on is provided in the request
		if req.CompletedOn != nil {
			if *req.CompletedOn == "" {
				// Empty string means mark as incomplete
				if err := d.readingRepository.UpdateCompletionDate(int(session.ID), document.ID, nil); err != nil {
					log.Printf("error marking document as incomplete: %s\n", err)
					return fiber.ErrInternalServerError
				}
			} else {
				// Parse the date string (expecting format: YYYY-MM-DD)
				completedOn, err := time.Parse("2006-01-02", *req.CompletedOn)
				if err != nil {
					return fiber.ErrBadRequest
				}

				// Prevent future dates - compare date components only
				now := time.Now()
				// Convert both to date-only format for comparison
				completedDateOnly := time.Date(completedOn.Year(), completedOn.Month(), completedOn.Day(), 0, 0, 0, 0, time.UTC)
				todayDateOnly := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

				if completedDateOnly.After(todayDateOnly) {
					return fiber.ErrBadRequest
				}

				// Update the completion date
				if err := d.readingRepository.UpdateCompletionDate(int(session.ID), document.ID, &completedOn); err != nil {
					log.Printf("error updating completion date: %s\n", err)
					return fiber.ErrInternalServerError
				}
			}
			return c.SendStatus(fiber.StatusNoContent)
		}
	}

	// No date provided - toggle behavior
	// Get current reading status to check if already completed
	reading, err := d.readingRepository.Get(int(session.ID), document.ID)
	if err != nil {
		// If no reading record exists yet, create one by touching it
		if err := d.readingRepository.Touch(int(session.ID), document.ID); err != nil {
			log.Printf("error creating reading record: %s\n", err)
			return fiber.ErrInternalServerError
		}
		reading.CompletedOn = nil
	}

	// Toggle completion status based on whether CompletedOn is set
	var newCompletionDate *time.Time
	if reading.CompletedOn == nil {
		// Not complete, mark as complete with current date
		now := time.Now()
		newCompletionDate = &now
	}
	// If reading.CompletedOn != nil, newCompletionDate stays nil (marking as incomplete)

	if err := d.readingRepository.UpdateCompletionDate(int(session.ID), document.ID, newCompletionDate); err != nil {
		log.Printf("error updating completion status: %s\n", err)
		return fiber.ErrInternalServerError
	}

	// Return 204 No Content - the client-side JavaScript will handle the UI update
	return c.SendStatus(fiber.StatusNoContent)
}
