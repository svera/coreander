package home

import (
	"log"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

func (d *Controller) Index(c fiber.Ctx) error {
	var session model.Session
	if val, ok := c.Locals("Session").(model.Session); ok {
		session = val
	}

	totalDocumentsCount, err := d.idx.Count()
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

		readingList, err := d.readingRepository.Latest(int(session.ID), 1, d.config.LatestDocsLimit)
		if err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
		for _, ID := range readingList.Hits() {
			doc, err := d.idx.DocumentByID(ID)
			if err != nil {
				log.Println(err)
				return fiber.ErrInternalServerError
			}
			if doc.ID == "" {
				continue
			}
			result := model.AugmentedDocument{Document: doc}
			result = d.hlRepository.Highlighted(int(session.ID), result)
			readingDocs = append(readingDocs, result)
		}
	}

	return c.Render("index", fiber.Map{
		"Count":      totalDocumentsCount,
		"EmailFrom":  d.sender.From(),
		"HomeNavbar": true,
		"LatestDocs": latestDocs,
		"Reading":    readingDocs,
	}, "layout")
}
