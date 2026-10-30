package document

import (
	"context"
	"log"
	"time"

	"github.com/gofiber/fiber/v3"
)

const reviewsTimeout = 30 * time.Second

// Score loads a document's score without blocking the detail page.
func (d *Controller) Score(c fiber.Ctx) error {
	if d.reviewProvider == nil {
		return c.SendString("")
	}
	document, err := d.idx.Document(c.Params("slug"))
	if err != nil {
		log.Println(err)
		return c.SendString("")
	}
	if document.Slug == "" {
		return c.SendString("")
	}
	ctx, cancel := context.WithTimeout(c.Context(), reviewsTimeout)
	defer cancel()
	score, err := d.reviewProvider.Score(ctx, document)
	if err != nil {
		log.Printf("Error retrieving score for %s: %s", document.ID, err)
		return c.SendString("")
	}
	if score.URL != "" && score.Value == "" {
		return c.SendString("")
	}
	return c.Render("partials/document-score", fiber.Map{
		"Source":       score.Source,
		"Rating":       score.Value,
		"RatingCount":  score.Count,
		"URL":          score.URL,
		"DocumentSlug": document.Slug,
	})
}
