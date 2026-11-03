package document

import (
	"errors"
	"log"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v5/internal/reviews"
)

func (d *Controller) UpdateDocumentLink(c fiber.Ctx) error {
	if d.reviewProvider == nil || !d.reviewProvider.CanEditDocumentLink() {
		return fiber.ErrNotFound
	}
	document, err := d.idx.Document(c.Params("slug"))
	if err != nil {
		log.Printf("Error reading document for link correction: %s", err)
		return fiber.ErrInternalServerError
	}
	if document.Slug == "" {
		return fiber.ErrNotFound
	}
	link := c.FormValue("bookURL")
	if err := d.reviewProvider.UpdateDocumentLink(document, link); err != nil {
		if errors.Is(err, reviews.ErrInvalidDocumentLink) {
			return c.Status(fiber.StatusUnprocessableEntity).Render("partials/document-review-link-form", fiber.Map{
				"Slug": document.Slug, "BookURL": link, "InvalidURL": true, "Source": d.reviewProvider.Name(),
			})
		}
		if errors.Is(err, reviews.ErrDocumentNotFound) {
			return fiber.ErrNotFound
		}
		log.Printf("Error correcting document link for %s: %s", document.ID, err)
		return fiber.ErrInternalServerError
	}
	c.Set("HX-Refresh", "true")
	return c.SendStatus(fiber.StatusNoContent)
}
