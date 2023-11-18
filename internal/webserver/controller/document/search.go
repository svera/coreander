package document

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v3/internal/index"
	"github.com/svera/coreander/v3/internal/result"
	"github.com/svera/coreander/v3/internal/webserver/infrastructure"
	"github.com/svera/coreander/v3/internal/webserver/jwtclaimsreader"
	"github.com/svera/coreander/v3/internal/webserver/model"
	"github.com/svera/coreander/v3/internal/webserver/view"
)

func (d *Controller) Search(c *fiber.Ctx) error {
	emailSendingConfigured := true
	if _, ok := d.sender.(*infrastructure.NoEmail); ok {
		emailSendingConfigured = false
	}

	page, err := strconv.Atoi(c.Query("page"))
	if err != nil {
		page = 1
	}

	session := jwtclaimsreader.SessionData(c)
	if session.WordsPerMinute > 0 {
		d.config.WordsPerMinute = session.WordsPerMinute
	}

	var searchResults result.Paginated[[]index.Document]

	if keywords := c.Query("search"); keywords != "" {
		if searchResults, err = d.idx.Search(keywords, page, model.ResultsPerPage); err != nil {
			return fiber.ErrInternalServerError
		}

		if session.ID > 0 {
			searchResults = d.hlRepository.HighlightedPaginatedResult(int(session.ID), searchResults)
		}

		return c.Render("results", fiber.Map{
			"Keywords":               keywords,
			"Results":                searchResults,
			"Paginator":              view.Pagination(model.MaxPagesNavigator, searchResults, map[string]string{"search": keywords}),
			"Title":                  "Search results",
			"EmailSendingConfigured": emailSendingConfigured,
			"EmailFrom":              d.sender.From(),
			"Session":                session,
			"WordsPerMinute":         d.config.WordsPerMinute,
		}, "layout")
	}

	count, err := d.idx.Count()
	if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.Render("index", fiber.Map{
		"Count":   count,
		"Title":   "Coreander",
		"Session": session,
	}, "layout")
}
