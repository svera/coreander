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

	if searchResults, err = a.idx.SearchBySeries(seriesSlug, page, model.ResultsPerPage); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	if searchResults.TotalHits() == 0 {
		return fiber.ErrNotFound
	}

	if session.ID > 0 {
		searchResults = a.hlRepository.HighlightedPaginatedResult(int(session.ID), searchResults)
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
