package controller

import (
	"fmt"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v3/internal/infrastructure"
	"github.com/svera/coreander/v3/internal/jwtclaimsreader"
	"github.com/svera/coreander/v3/internal/metadata"
)

func Detail(c *fiber.Ctx, libraryPath string, sender Sender, idx IdxReader, wordsPerMinute float64) error {
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

	if _, err := os.Stat(fmt.Sprintf("%s%s%s", libraryPath, string(os.PathSeparator), document.ID)); err != nil {
		return fiber.ErrNotFound
	}

	title := fmt.Sprintf("%s | Coreander", document.Title)
	authors := strings.Join(document.Authors, ", ")
	if authors != "" {
		title = fmt.Sprintf("%s - %s | Coreander", authors, document.Title)
	}

	return c.Render("detail", fiber.Map{
		"Lang":                   lang,
		"Title":                  title,
		"Document":               document,
		"EmailSendingConfigured": emailSendingConfigured,
		"EmailFrom":              sender.From(),
		"Session":                session,
		"ReadingTime":            metadata.CalculateReadingTime(document.Words, wordsPerMinute),
	}, "layout")

}
