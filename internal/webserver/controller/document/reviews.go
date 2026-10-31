package document

import (
	"context"
	"html/template"
	"log"
	"math"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v5/internal/metadata"
	"github.com/svera/coreander/v5/internal/reviews"
	"golang.org/x/exp/slices"
)

type documentReview struct {
	reviews.Review
	HTML      template.HTML
	CreatedOn string
	StarFills [5]int
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
	reviewLanguage := c.Query("language")
	supportedLanguages := c.Locals("SupportedLanguages").([]string)
	if reviewLanguage == "" {
		reviewLanguage = lang
	}
	if !slices.Contains(supportedLanguages, reviewLanguage) {
		return fiber.ErrBadRequest
	}
	page, err := d.reviewProvider.Reviews(ctx, document, reviewLanguage)
	if err != nil {
		log.Printf("Error retrieving reviews for %s: %s", document.ID, err)
		return fiber.ErrInternalServerError
	}
	if !page.Available {
		link := d.documentServiceLink(document)
		if link.URL == "" {
			return c.SendString("")
		}
		return c.Render("partials/document-reviews", fiber.Map{
			"Lang": lang, "Slug": document.Slug, "ReviewLanguage": reviewLanguage,
			"SupportedLanguages": supportedLanguages, "Source": link.Source,
			"BookURL": link.URL, "Unavailable": true,
		})
	}
	renderedReviews := make([]documentReview, 0, len(page.Reviews))
	for _, review := range page.Reviews {
		var createdOn string
		if !review.CreatedAt.IsZero() {
			createdOn = review.CreatedAt.Format(time.DateOnly)
		}
		rendered := documentReview{
			Review:    review,
			HTML:      template.HTML(metadata.SanitizeDescription(review.Text)),
			CreatedOn: createdOn,
		}
		for point := range rendered.StarFills {
			rendered.StarFills[point] = int(math.Round(min(1, max(0, review.Rating-float64(point))) * 100))
		}
		renderedReviews = append(renderedReviews, rendered)
	}
	return c.Render("partials/document-reviews", fiber.Map{
		"Lang": lang, "Slug": document.Slug, "ReviewLanguage": reviewLanguage,
		"SupportedLanguages": supportedLanguages, "Reviews": renderedReviews,
		"Source": page.Source, "BookURL": page.URL,
	})
}
