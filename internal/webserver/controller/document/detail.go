package document

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v3/internal/infrastructure"
	"github.com/svera/coreander/v3/internal/webserver/jwtclaimsreader"
)

func (d *Controller) Detail(c *fiber.Ctx) error {
	emailSendingConfigured := true
	if _, ok := d.sender.(*infrastructure.NoEmail); ok {
		emailSendingConfigured = false
	}

	session := jwtclaimsreader.SessionData(c)
	if session.WordsPerMinute > 0 {
		d.config.WordsPerMinute = session.WordsPerMinute
	}

	document, err := d.idx.Document(c.Params("slug"))
	if err != nil {
		fmt.Println(err)
		return fiber.ErrBadRequest
	}

	if _, err := os.Stat(filepath.Join(d.config.LibraryPath, document.ID)); err != nil {
		return fiber.ErrNotFound
	}

	title := fmt.Sprintf("%s | Coreander", document.Title)
	authors := strings.Join(document.Authors, ", ")
	if authors != "" {
		title = fmt.Sprintf("%s - %s | Coreander", authors, document.Title)
	}

	sameSubjects, err := d.idx.SameSubjects(document.Slug, relatedDocuments)
	if err != nil {
		fmt.Println(err)
	}

	sameAuthors, err := d.idx.SameAuthors(document.Slug, relatedDocuments)
	if err != nil {
		fmt.Println(err)
	}

	sameSeries, err := d.idx.SameSeries(document.Slug, relatedDocuments)
	if err != nil {
		fmt.Println(err)
	}

	if session.ID > 0 {
		document = d.hlRepository.Highlighted(int(session.ID), document)
	}

	return c.Render("document", fiber.Map{
		"Title":                  title,
		"Document":               document,
		"EmailSendingConfigured": emailSendingConfigured,
		"EmailFrom":              d.sender.From(),
		"Session":                session,
		"SameSeries":             sameSeries,
		"SameAuthors":            sameAuthors,
		"SameSubjects":           sameSubjects,
		"WordsPerMinute":         d.config.WordsPerMinute,
	}, "layout")
}
