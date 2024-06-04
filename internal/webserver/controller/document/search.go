package document

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
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

	page, err := strconv.Atoi(c.Query("page"))
	if err != nil {
		page = 1
	}

	var session model.Session
	if val, ok := c.Locals("Session").(model.Session); ok {
		session = val
	}

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
			"WordsPerMinute":         d.config.WordsPerMinute,
		}, "layout")
	}

	count, err := d.idx.Count()
	if err != nil {
		return fiber.ErrInternalServerError
	}

	docsSortedByHighlightedDate, err := d.hlRepository.Highlights(int(session.ID), page, 6)
	if err != nil {
		return fiber.ErrInternalServerError
	}

	docs, err := d.idx.Documents(docsSortedByHighlightedDate.Hits())
	if err != nil {
		return fiber.ErrInternalServerError
	}

	highlights := make([]index.Document, 0, len(docs))
	for _, path := range docsSortedByHighlightedDate.Hits() {
		if _, ok := docs[path]; !ok {
			continue
		}
		doc := docs[path]
		doc.Highlighted = true
		highlights = append(highlights, doc)
	}

	return c.Render("index", fiber.Map{
		"Count":                  count,
		"Title":                  "Coreander",
		"Highlights":             highlights,
		"EmailSendingConfigured": emailSendingConfigured,
		"EmailFrom":              d.sender.From(),
	}, "layout")
}
