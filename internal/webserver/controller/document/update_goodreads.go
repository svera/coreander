package document

import (
	"errors"
	"log"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v5/internal/index"
)

func (d *Controller) UpdateGoodReads(c fiber.Ctx) error {
	document, err := d.idx.Document(c.Params("slug"))
	if err != nil {
		log.Printf("Error reading document for Goodreads correction: %s", err)
		return fiber.ErrInternalServerError
	}
	if document.Slug == "" {
		return fiber.ErrNotFound
	}
	bookURL := c.FormValue("bookURL")
	if err := d.idx.UpdateGoodReadsBookURL(document.ID, bookURL); err != nil {
		if errors.Is(err, index.ErrInvalidGoodReadsBookURL) {
			return c.Status(fiber.StatusUnprocessableEntity).Render("partials/document-goodreads-form", fiber.Map{
				"Lang": c.Locals("Lang"), "Slug": document.Slug, "BookURL": bookURL, "InvalidURL": true,
			})
		}
		if errors.Is(err, index.ErrDocumentNotFound) {
			return fiber.ErrNotFound
		}
		log.Printf("Error correcting Goodreads link for %s: %s", document.ID, err)
		return fiber.ErrInternalServerError
	}
	c.Set("HX-Refresh", "true")
	return c.SendStatus(fiber.StatusNoContent)
}
