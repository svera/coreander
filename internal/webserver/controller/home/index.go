package home

import (
	"log"

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

	totalDocumentsCount, err := d.idx.Count(index.TypeDocument)
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	latestDocs, err := d.idx.LatestDocs(d.config.LatestDocsLimit)
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	var readingDocs []index.Document
	if session.ID > 0 {
		for i := range latestDocs {
			latestDocs[i] = d.hlRepository.Highlighted(int(session.ID), latestDocs[i])
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
			readingDocs = append(readingDocs, doc)
		}
	}

	return c.Render("index", fiber.Map{
		"Count":                  totalDocumentsCount,
		"EmailSendingConfigured": emailSendingConfigured,
		"EmailFrom":              d.sender.From(),
		"HomeNavbar":             true,
		"LatestDocs":             latestDocs,
		"Reading":                readingDocs,
	}, "layout")
}
