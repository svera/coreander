package document

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/controller"
	"github.com/svera/coreander/v4/internal/infrastructure"
	"github.com/svera/coreander/v4/internal/jwtclaimsreader"
	"github.com/svera/coreander/v4/internal/model"
	"github.com/svera/coreander/v4/internal/search"
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

	var searchResults *search.PaginatedResult

	if keywords := c.Query("search"); keywords != "" {
		if searchResults, err = d.idx.Search(keywords, page, model.ResultsPerPage); err != nil {
			return fiber.ErrInternalServerError
		}

		if session.ID > 0 {
			searchResults.Hits = d.hlRepository.Highlighted(int(session.ID), searchResults.Hits)
		}

		return c.Render("results", fiber.Map{
			"Keywords":               keywords,
			"Results":                searchResults.Hits,
			"Total":                  searchResults.TotalHits,
			"Paginator":              controller.Pagination(model.MaxPagesNavigator, searchResults.TotalPages, searchResults.Page, map[string]string{"search": keywords}),
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
