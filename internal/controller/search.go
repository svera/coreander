package controller

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v3/internal/infrastructure"
	"github.com/svera/coreander/v3/internal/jwtclaimsreader"
	"github.com/svera/coreander/v3/internal/model"
	"github.com/svera/coreander/v3/internal/search"
)

type Sender interface {
	SendDocument(address string, libraryPath string, fileName string) error
	From() string
}

// IdxReader defines a set of reading operations over an index
type IdxReader interface {
	Search(keywords string, page, resultsPerPage int) (*search.PaginatedResult, error)
	Count() (uint64, error)
	Close() error
	Document(Slug string) (search.Document, error)
	Documents(IDs []string) ([]search.Document, error)
	SameSubjects(slug string, quantity int) ([]search.Document, error)
	SameAuthors(slug string, quantity int) ([]search.Document, error)
	SameSeries(slug string, quantity int) ([]search.Document, error)
}

func Search(c *fiber.Ctx, idx IdxReader, sender Sender, wordsPerMinute float64, highlights model.HighlightRepository) error {
	emailSendingConfigured := true
	if _, ok := sender.(*infrastructure.NoEmail); ok {
		emailSendingConfigured = false
	}

	page, err := strconv.Atoi(c.Query("page"))
	if err != nil {
		page = 1
	}

	session := jwtclaimsreader.SessionData(c)
	if session.WordsPerMinute > 0 {
		wordsPerMinute = session.WordsPerMinute
	}

	var searchResults *search.PaginatedResult

	if keywords := c.Query("search"); keywords != "" {
		if searchResults, err = idx.Search(keywords, page, model.ResultsPerPage); err != nil {
			return fiber.ErrInternalServerError
		}

		if session.ID > 0 {
			searchResults.Hits = highlights.Highlighted(int(session.ID), searchResults.Hits)
		}

		return c.Render("results", fiber.Map{
			"Keywords":               keywords,
			"Results":                searchResults.Hits,
			"Total":                  searchResults.TotalHits,
			"Paginator":              pagination(model.MaxPagesNavigator, searchResults.TotalPages, searchResults.Page, map[string]string{"search": keywords}),
			"Title":                  "Search results",
			"EmailSendingConfigured": emailSendingConfigured,
			"EmailFrom":              sender.From(),
			"Session":                session,
			"WordsPerMinute":         wordsPerMinute,
		}, "layout")
	}

	count, err := idx.Count()
	if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.Render("index", fiber.Map{
		"Count":   count,
		"Title":   "Coreander",
		"Session": session,
	}, "layout")
}
