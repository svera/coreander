package document

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

func (d *Controller) Detail(c *fiber.Ctx) error {
	emailSendingConfigured := true
	if _, ok := d.sender.(*infrastructure.NoEmail); ok {
		emailSendingConfigured = false
	}

	var session model.Session
	if val, ok := c.Locals("Session").(model.Session); ok {
		session = val
	}

	if session.WordsPerMinute > 0 {
		d.config.WordsPerMinute = session.WordsPerMinute
	}

	document, err := d.idx.Document(c.Params("slug"))
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	if document.Slug == "" {
		return fiber.ErrNotFound
	}

	if _, err := os.Stat(filepath.Join(d.config.LibraryPath, document.ID)); err != nil {
		return fiber.ErrNotFound
	}

	title := fmt.Sprintf("%s | Coreander", document.Title)
	if len(document.Authors) > 0 {
		title = fmt.Sprintf("%s - %s | Coreander", strings.Join(document.Authors, ", "), document.Title)
	}

	sameSubjects, sameAuthors, sameSeries := d.related(document.Slug, (int(session.ID)))

	if session.ID > 0 {
		document = d.hlRepository.Highlighted(int(session.ID), document)
	}

	msg := ""
	if c.Query("success") != "" {
		msg = "Document uploaded successfully."
	}

	return c.Render("document", fiber.Map{
		"Title":                  title,
		"Document":               document,
		"EmailSendingConfigured": emailSendingConfigured,
		"EmailFrom":              d.sender.From(),
		"SameSeries":             sameSeries,
		"SameAuthors":            sameAuthors,
		"SameSubjects":           sameSubjects,
		"WordsPerMinute":         d.config.WordsPerMinute,
		"Message":                msg,
	}, "layout")
}

func (d *Controller) related(slug string, sessionID int) (sameSubjects, sameAuthors, sameSeries []index.Document) {
	var err error
	sameSubjects, err = d.idx.SameSubjects(slug, relatedDocuments)
	if err != nil {
		fmt.Println(err)
	}
	for i := range sameSubjects {
		sameSubjects[i] = d.hlRepository.Highlighted(sessionID, sameSubjects[i])
	}

	sameAuthors, err = d.idx.SameAuthors(slug, relatedDocuments)
	if err != nil {
		fmt.Println(err)
	}
	for i := range sameAuthors {
		sameAuthors[i] = d.hlRepository.Highlighted(sessionID, sameAuthors[i])
	}

	sameSeries, err = d.idx.SameSeries(slug, relatedDocuments)
	if err != nil {
		fmt.Println(err)
	}
	for i := range sameSeries {
		sameSeries[i] = d.hlRepository.Highlighted(sessionID, sameSeries[i])
	}
	return sameSubjects, sameAuthors, sameSeries
}
