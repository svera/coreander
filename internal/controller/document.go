package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v3/internal/infrastructure"
	"github.com/svera/coreander/v3/internal/jwtclaimsreader"
	"github.com/svera/coreander/v3/internal/model"
	"github.com/svera/coreander/v3/internal/search"
)

const relatedDocuments = 4

func Document(c *fiber.Ctx, libraryPath string, sender Sender, idx IdxReader, wordsPerMinute float64, highlights model.HighlightRepository) error {
	emailSendingConfigured := true
	if _, ok := sender.(*infrastructure.NoEmail); ok {
		emailSendingConfigured = false
	}

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

	if session.ID > 0 {
		document = highlights.Highlighted(int(session.ID), []search.Document{document})[0]
	}

	return c.Render("document", fiber.Map{
		"Title":                  title,
		"Document":               document,
		"EmailSendingConfigured": emailSendingConfigured,
		"EmailFrom":              sender.From(),
		"Session":                session,
		"SameSeries":             sameSeries,
		"SameAuthors":            sameAuthors,
		"SameSubjects":           sameSubjects,
		"WordsPerMinute":         wordsPerMinute,
	}, "layout")

}
