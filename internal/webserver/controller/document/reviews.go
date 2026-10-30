package document

import (
	"context"
	"html/template"
	"log"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v5/internal/metadata"
	"github.com/svera/coreander/v5/internal/reviews"
)

type documentReview struct {
	reviews.Review
	HTML      template.HTML
	CreatedOn string
}

// Reviews loads the first page of user reviews when the detail page's bottom is visible.
func (d *Controller) Reviews(c fiber.Ctx) error {
	if d.reviewProvider == nil {
		return c.SendString("")
	}

	document, err := d.idx.Document(c.Params("slug"))
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	if document.Slug == "" {
		return fiber.ErrNotFound
	}
	ctx, cancel := context.WithTimeout(c.Context(), reviewsTimeout)
	defer cancel()
	lang := c.Locals("Lang").(string)
	page, err := d.reviewProvider.Reviews(ctx, document, lang)
	if err != nil {
		log.Printf("Error retrieving reviews for %s: %s", document.ID, err)
		return fiber.ErrInternalServerError
	}
	if !page.Available {
		return c.SendString("")
	}
	renderedReviews := make([]documentReview, 0, len(page.Reviews))
	for _, review := range page.Reviews {
		var createdOn string
		if !review.CreatedAt.IsZero() {
			createdOn = review.CreatedAt.Format(time.DateOnly)
		}
		renderedReviews = append(renderedReviews, documentReview{
			Review:    review,
			HTML:      template.HTML(metadata.SanitizeDescription(review.Text)),
			CreatedOn: createdOn,
		})
	}
	return c.Render("partials/document-reviews", fiber.Map{
		"Reviews": renderedReviews,
		"Source":  page.Source,
		"BookURL": page.URL,
	})
}
