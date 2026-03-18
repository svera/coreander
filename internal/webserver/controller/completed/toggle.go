package completed

import (
	"log"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

type updateCompletionDateRequest struct {
	CompletedOn *string `json:"completed_on"`
}

// ToggleComplete marks a document as complete or incomplete.
// If a date is provided in the request body, it sets the completion date to that value.
// If no date is provided (POST), it toggles between complete (with current date) and incomplete.
func (c *Controller) ToggleComplete(ctx fiber.Ctx) error {
	var session model.Session
	if val, ok := ctx.Locals("Session").(model.Session); ok {
		session = val
	}

	if session.ID == 0 {
		return fiber.ErrUnauthorized
	}

	document, err := c.idxReader.Document(ctx.Params("slug"))
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	if document.Slug == "" {
		return fiber.ErrNotFound
	}

	var req updateCompletionDateRequest
	if ctx.Body() != nil && len(ctx.Body()) > 0 {
		if err := ctx.Bind().Body(&req); err != nil {
			return fiber.ErrBadRequest
		}

		if req.CompletedOn != nil {
			if *req.CompletedOn == "" {
				if err := c.readingRepository.UpdateCompletionDate(int(session.ID), document.Slug, nil); err != nil {
					log.Printf("error marking document as incomplete: %s\n", err)
					return fiber.ErrInternalServerError
				}
			} else {
				completedOn, err := time.Parse("2006-01-02", *req.CompletedOn)
				if err != nil {
					return fiber.ErrBadRequest
				}

				now := time.Now()
				completedDateOnly := time.Date(completedOn.Year(), completedOn.Month(), completedOn.Day(), 0, 0, 0, 0, time.UTC)
				todayDateOnly := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

				if completedDateOnly.After(todayDateOnly) {
					return fiber.ErrBadRequest
				}

				if err := c.readingRepository.UpdateCompletionDate(int(session.ID), document.Slug, &completedOn); err != nil {
					log.Printf("error updating completion date: %s\n", err)
					return fiber.ErrInternalServerError
				}
			}
			return ctx.SendStatus(fiber.StatusNoContent)
		}
	}

	reading, err := c.readingRepository.Get(int(session.ID), document.Slug)
	if err != nil {
		if err := c.readingRepository.Touch(int(session.ID), document.Slug); err != nil {
			log.Printf("error creating reading record: %s\n", err)
			return fiber.ErrInternalServerError
		}
		reading.CompletedOn = nil
	}

	var newCompletionDate *time.Time
	if reading.CompletedOn == nil {
		now := time.Now()
		newCompletionDate = &now
	}

	if err := c.readingRepository.UpdateCompletionDate(int(session.ID), document.Slug, newCompletionDate); err != nil {
		log.Printf("error updating completion status: %s\n", err)
		return fiber.ErrInternalServerError
	}

	return ctx.SendStatus(fiber.StatusNoContent)
}
