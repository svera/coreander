package series

import (
	"log"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/result"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"github.com/svera/coreander/v4/internal/webserver/model"
	"github.com/svera/coreander/v4/internal/webserver/view"
)

func (a *Controller) Documents(c *fiber.Ctx) error {
	emailSendingConfigured := true
	if _, ok := a.sender.(*infrastructure.NoEmail); ok {
		emailSendingConfigured = false
	}

	var session model.Session
	if val, ok := c.Locals("Session").(model.Session); ok {
		session = val
	}

	if session.WordsPerMinute > 0 {
		a.config.WordsPerMinute = session.WordsPerMinute
	}

	var searchResults result.Paginated[[]index.Document]
	seriesSlug := c.Params("slug")

	if seriesSlug == "" {
		return fiber.ErrBadRequest
	}

	page, err := strconv.Atoi(c.Query("page"))
	if err != nil {
		page = 1
	}

	searchFields := index.SearchFields{
		Keywords: seriesSlug,
		SortBy:   a.parseSortBy(c),
	}

	if searchResults, err = a.idx.SearchBySeries(searchFields, page, model.ResultsPerPage); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	if searchResults.TotalHits() == 0 {
		return fiber.ErrNotFound
	}

	if session.ID > 0 {
		searchResults = a.hlRepository.HighlightedPaginatedResult(int(session.ID), searchResults)
		searchResults = a.readingRepository.CompletedPaginatedResult(int(session.ID), searchResults)
	}

	title := searchResults.Hits()[0].Series

	templateVars := fiber.Map{
		"Results":                searchResults,
		"Paginator":              view.Pagination(model.MaxPagesNavigator, searchResults, c.Queries()),
		"Title":                  title,
		"EmailSendingConfigured": emailSendingConfigured,
		"EmailFrom":              a.sender.From(),
		"WordsPerMinute":         a.config.WordsPerMinute,
		"URL":                    view.URL(c),
		"SortURL":                view.SortURL(c),
		"SortBy":                 c.Query("sort-by"),
		"AvailableLanguages":     c.Locals("AvailableLanguages"),
		"AdditionalSortOptions": []struct {
			Key   string
			Value string
		}{
			{"number", "series number"},
			{"number-desc", "series number (descending)"},
			{"pub-date-older-first", "older"},
			{"pub-date-newer-first", "newer"},
			{"est-read-time-shorter-first", "shorter"},
			{"est-read-time-longer-first", "longer"},
		},
	}

	if c.Get("hx-request") == "true" {
		if err = c.Render("partials/docs-list", templateVars); err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
		return nil
	}

	if err = c.Render("series/results", templateVars, "layout"); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	return nil
}

func (d *Controller) parseSortBy(c *fiber.Ctx) []string {
	if c.Query("sort-by") != "" {
		switch c.Query("sort-by") {
		case "pub-date-older-first":
			return []string{"Publication.Date", "SeriesIndex"}
		case "pub-date-newer-first":
			return []string{"-Publication.Date", "SeriesIndex"}
		case "number-desc":
			return []string{"-SeriesIndex"}
		case "est-read-time-shorter-first":
			return []string{"Words"}
		case "est-read-time-longer-first":
			return []string{"-Words"}
		}
	}
	return []string{"SeriesIndex"}
}
