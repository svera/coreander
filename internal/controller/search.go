package controller

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v3/internal/infrastructure"
	"github.com/svera/coreander/v3/internal/jwtclaimsreader"
	"github.com/svera/coreander/v3/internal/metadata"
	"github.com/svera/coreander/v3/internal/model"
)

// Result holds the result of a search request, as well as some related metadata
type Result struct {
	Page       int
	TotalPages int
	Hits       []metadata.Metadata
	TotalHits  int
}

type Sender interface {
	SendDocument(address string, libraryPath string, fileName string) error
	From() string
}

// Reader defines a set of reading operations over an index
type Reader interface {
	Search(keywords string, page, resultsPerPage int, wordsPerMinute float64) (*Result, error)
	Count() (uint64, error)
	Close() error
	Document(ID string) (metadata.Metadata, error)
}

func Search(c *fiber.Ctx, idx Reader, version string, sender Sender, wordsPerMinute float64) error {
	emailSendingConfigured := true
	if _, ok := sender.(*infrastructure.NoEmail); ok {
		emailSendingConfigured = false
	}

	page, err := strconv.Atoi(c.Query("page"))
	if err != nil {
		page = 1
	}

	session := jwtclaimsreader.SessionData(c)
	var searchResults *Result

	if keywords := c.Query("search"); keywords != "" {
		if searchResults, err = idx.Search(keywords, page, model.ResultsPerPage, wordsPerMinute); err != nil {
			return fiber.ErrInternalServerError
		}

		return c.Render("results", fiber.Map{
			"Keywords":               keywords,
			"Results":                searchResults.Hits,
			"Total":                  searchResults.TotalHits,
			"Paginator":              pagination(model.MaxPagesNavigator, searchResults.TotalPages, searchResults.Page, map[string]string{"search": keywords}),
			"Title":                  "Search results",
			"Version":                version,
			"EmailSendingConfigured": emailSendingConfigured,
			"EmailFrom":              sender.From(),
			"Session":                session,
		}, "layout")
	}
	count, err := idx.Count()
	if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.Render("index", fiber.Map{
		"Count":   count,
		"Title":   "Coreander",
		"Version": version,
		"Session": session,
	}, "layout")
}
