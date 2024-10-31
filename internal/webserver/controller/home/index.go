package home

import (
	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

func (d *Controller) Index(c *fiber.Ctx) error {
	emailSendingConfigured := true
	if _, ok := d.sender.(*infrastructure.NoEmail); ok {
		emailSendingConfigured = false
	}

	var session model.Session
	if val, ok := c.Locals("Session").(model.Session); ok {
		session = val
	}

	count, err := d.idx.Count()
	if err != nil {
		return fiber.ErrInternalServerError
	}

	docsSortedByHighlightedDate, err := d.hlRepository.Highlights(int(session.ID), 0, 6)
	if err != nil {
		return fiber.ErrInternalServerError
	}

	docs, err := d.idx.Documents(docsSortedByHighlightedDate.Hits())
	if err != nil {
		return fiber.ErrInternalServerError
	}

	highlights := make([]index.Document, 0, len(docs))
	for _, path := range docsSortedByHighlightedDate.Hits() {
		if _, ok := docs[path]; !ok {
			continue
		}
		doc := docs[path]
		doc.Highlighted = true
		highlights = append(highlights, doc)
	}

	return c.Render("index", fiber.Map{
		"Count":                  count,
		"Title":                  "Coreander",
		"Highlights":             highlights,
		"EmailSendingConfigured": emailSendingConfigured,
		"EmailFrom":              d.sender.From(),
		"HomeNavbar":             true,
	}, "layout")
}
