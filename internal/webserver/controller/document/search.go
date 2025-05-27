package document

import (
	"fmt"
	"log"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/rickb777/date/v2"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/result"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"github.com/svera/coreander/v4/internal/webserver/model"
	"github.com/svera/coreander/v4/internal/webserver/view"
)

func (d *Controller) Search(c *fiber.Ctx) error {
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

	var searchResults result.Paginated[[]index.Document]
	searchFields, err := d.parseSearchQuery(c)
	if err != nil {
		log.Println(err)
		return fiber.ErrBadRequest
	}

	page, err := strconv.Atoi(c.Query("page"))
	if err != nil {
		page = 1
	}

	if searchResults, err = d.idx.Search(searchFields, page, model.ResultsPerPage); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	if session.ID > 0 {
		searchResults = d.hlRepository.HighlightedPaginatedResult(int(session.ID), searchResults)
	}

	queries := c.Queries()
	delete(queries, "page")
	templateVars := fiber.Map{
		"SearchFields":           searchFields,
		"Results":                searchResults,
		"Paginator":              view.Pagination(model.MaxPagesNavigator, searchResults, queries),
		"Title":                  "Search results",
		"EmailSendingConfigured": emailSendingConfigured,
		"EmailFrom":              d.sender.From(),
		"WordsPerMinute":         d.config.WordsPerMinute,
		"URL":                    view.URL(c),
	}

	if c.Get("hx-request") == "true" {
		if err = c.Render("partials/docs-list", templateVars); err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
		return nil
	}

	if err = c.Render("document/results", templateVars, "layout"); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	return nil
}

func (d *Controller) parseSearchQuery(c *fiber.Ctx) (index.SearchFields, error) {
	searchFields := index.SearchFields{
		Keywords: c.Query("search"),
	}

	/*if searchFields.Keywords == "" {
		return searchFields, fmt.Errorf("search keywords cannot be empty")
	}*/

	if c.Query("pub-date-from-year") != "" && c.Query("pub-date-from-month") != "" && c.Query("pub-date-from-day") != "" {
		pubDateFrom, err := date.ParseISO(fmt.Sprintf("%04s-%02s-%02s", c.Query("pub-date-from-year"), c.Query("pub-date-from-month"), c.Query("pub-date-from-day")))
		if err != nil {
			return searchFields, err
		}
		searchFields.PubDateFrom = pubDateFrom
	}

	if c.Query("pub-date-to-year") != "" && c.Query("pub-date-to-month") != "" && c.Query("pub-date-to-day") != "" {
		pubDateTo, err := date.ParseISO(fmt.Sprintf("%04s-%02s-%02s", c.Query("pub-date-to-year"), c.Query("pub-date-to-month"), c.Query("pub-date-to-day")))
		if err != nil {
			return searchFields, err
		}
		searchFields.PubDateTo = pubDateTo
	}

	if searchFields.PubDateTo != 0 && searchFields.PubDateFrom > searchFields.PubDateTo {
		return searchFields, fmt.Errorf("publication date from cannot be later than publication date to")
	}

	return searchFields, nil
}
