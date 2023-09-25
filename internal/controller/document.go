package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v3/internal/infrastructure"
	"github.com/svera/coreander/v3/internal/jwtclaimsreader"
)

const relatedDocuments = 4

func Document(c *fiber.Ctx, libraryPath string, sender Sender, idx IdxReader, wordsPerMinute float64) error {
	emailSendingConfigured := true
	if _, ok := sender.(*infrastructure.NoEmail); ok {
		emailSendingConfigured = false
	}

	lang := c.Params("lang")
	session := jwtclaimsreader.SessionData(c)
	if session.WordsPerMinute > 0 {
		wordsPerMinute = session.WordsPerMinute
	}

	document, err := idx.Document(c.Params("slug"))
	if err != nil {
		fmt.Println(err)
		return fiber.ErrBadRequest
	}

	if _, err := os.Stat(filepath.Join(libraryPath, document.ID)); err != nil {
		return fiber.ErrNotFound
	}

	title := fmt.Sprintf("%s | Coreander", document.Title)
	authors := strings.Join(document.Authors, ", ")
	if authors != "" {
		title = fmt.Sprintf("%s - %s | Coreander", authors, document.Title)
	}

	sameSubjects, err := idx.SameSubjects(document.Slug, relatedDocuments)
	if err != nil {
		fmt.Println(err)
	}

	sameAuthors, err := idx.SameAuthors(document.Slug, relatedDocuments)
	if err != nil {
		fmt.Println(err)
	}

	sameSeries, err := idx.SameSeries(document.Slug, relatedDocuments)
	if err != nil {
		fmt.Println(err)
	}

	return c.Render("document", fiber.Map{
		"Lang":                   lang,
		"Title":                  title,
		"Document":               document,
		"Authors":                strings.Join(document.Authors, ","),
		"Subjects":               strings.Join(document.Subjects, ","),
		"EmailSendingConfigured": emailSendingConfigured,
		"EmailFrom":              sender.From(),
		"Session":                session,
		"SameSeries":             sameSeries,
		"SameAuthors":            sameAuthors,
		"SameSubjects":           sameSubjects,
		"WordsPerMinute":         wordsPerMinute,
		"Version":                c.App().Config().AppName,
	}, "layout")

}
