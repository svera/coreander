package document

import (
	"log"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/webserver/model"
)

// Similar renders the "similar documents" section of a document's detail
// page. It's loaded asynchronously via htmx so the similarity query, which
// can be expensive on large libraries, doesn't block the initial page render.
func (d *Controller) Similar(c fiber.Ctx) error {
	var session model.Session
	if val, ok := c.Locals("Session").(model.Session); ok {
		session = val
	}

	docSlug := c.Params("slug")
	if docSlug == "" {
		return fiber.ErrBadRequest
	}

	similar, err := d.idx.Search(index.SearchFields{
		SimilarTo: docSlug,
		SortBy:    index.DefaultDocumentSortBy,
	}, 1, relatedDocuments)
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	var similarDocuments []model.AugmentedDocument
	for _, document := range similar.Hits() {
		result := model.AugmentedDocument{Document: document}
		result = d.hlRepository.Highlighted(int(session.ID), result)
		similarDocuments = append(similarDocuments, result)
	}

	return c.Render("partials/document-similar", fiber.Map{
		"DocumentSlug":          docSlug,
		"SimilarDocuments":      similarDocuments,
		"SimilarDocumentsTotal": similar.TotalHits(),
	})
}
