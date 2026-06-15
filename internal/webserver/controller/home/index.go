package home

import (
	"log"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v5/internal/webserver/model"
)

func (d *Controller) Index(c fiber.Ctx) error {
	if query := strings.TrimSpace(c.Query("search")); query != "" {
		return c.Redirect().To("/search?type=documents&search=" + url.QueryEscape(query))
	}

	var session model.Session
	if val, ok := c.Locals("Session").(model.Session); ok {
		session = val
	}

	totalDocumentsCount, err := d.idx.Count()
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	totalAuthorsCount, err := d.idx.AuthorsCount()
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	latestDocsRaw, err := d.idx.LatestDocs(d.config.LatestDocsLimit)
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	latestDocs := make([]model.AugmentedDocument, 0, len(latestDocsRaw))
	for _, doc := range latestDocsRaw {
		latestDocs = append(latestDocs, model.AugmentedDocument{Document: doc})
	}

	var readingDocs []model.AugmentedDocument
	if session.ID > 0 {
		for i := range latestDocs {
			result := model.AugmentedDocument{Document: latestDocs[i].Document}
			result = d.hlRepository.Highlighted(int(session.ID), result)
			latestDocs[i] = result
		}

		readingDocs, err = d.readingDocs(int(session.ID))
		if err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
	}

	return c.Render("index", fiber.Map{
		"Count":        totalDocumentsCount,
		"AuthorsCount": totalAuthorsCount,
		"EmailFrom":    d.sender.From(),
		"HomeNavbar":   true,
		"LatestDocs":   latestDocs,
		"Reading":      readingDocs,
	}, "layout")
}
