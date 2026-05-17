package home

import (
	"log"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

func (d *Controller) readingDocs(userID int) ([]model.AugmentedDocument, error) {
	readingPage, err := d.readingRepository.Latest(userID, 1, d.config.LatestDocsLimit)
	if err != nil {
		return nil, err
	}

	readingDocs := make([]model.AugmentedDocument, 0, len(readingPage.Hits()))
	for _, doc := range readingPage.Hits() {
		result := d.hlRepository.Highlighted(userID, doc)
		readingDocs = append(readingDocs, result)
	}
	return readingDocs, nil
}

func (d *Controller) ResumeReading(c fiber.Ctx) error {
	c.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Set("Pragma", "no-cache")

	session, _ := c.Locals("Session").(model.Session)

	readingDocs, err := d.readingDocs(int(session.ID))
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	return c.Render("partials/resume-reading", fiber.Map{
		"Reading": readingDocs,
		"Compact": c.Query("compact") == "true",
	})
}
