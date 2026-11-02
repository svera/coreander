package document

import (
	"context"
	"log"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v5/internal/webserver/model"
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
	session, _ := c.Locals("Session").(model.Session)
	canEditDocumentLink := session.Role == model.RoleAdmin && d.reviewProvider.CanEditDocumentLink()
	if err != nil {
		log.Printf("Error retrieving score for %s: %s", document.ID, err)
		if !canEditDocumentLink {
			return c.SendString("")
		}
		score.Value, score.Count = "", ""
		score.URL = d.documentServiceLink(document).URL
	}
	return c.Render("partials/document-score", fiber.Map{
		"Source":              score.Source,
		"Rating":              score.Value,
		"RatingCount":         score.Count,
		"Scored":              score.HasScore(),
		"URL":                 score.URL,
		"DocumentSlug":        document.Slug,
		"CanEditDocumentLink": canEditDocumentLink,
		"ScoreUnavailable":    err != nil,
	})
}
