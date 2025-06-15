package author

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
	authorSlug := c.Params("slug")

	if authorSlug == "" {
		return fiber.ErrBadRequest
	}

	page, err := strconv.Atoi(c.Query("page"))
	if err != nil {
		page = 1
	}

	author, err := a.idx.Author(authorSlug, c.Locals("Lang").(string))
	if err != nil {
		log.Println(err)
	}

	searchFields := index.SearchFields{
		Keywords: authorSlug,
		SortBy:   a.parseSortBy(c),
	}

	if searchResults, err = a.idx.SearchByAuthor(searchFields, page, model.ResultsPerPage); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	if session.ID > 0 {
		searchResults = a.hlRepository.HighlightedPaginatedResult(int(session.ID), searchResults)
	}

	templateVars := fiber.Map{
		"Author":                 author,
		"Results":                searchResults,
		"Paginator":              view.Pagination(model.MaxPagesNavigator, searchResults, c.Queries()),
		"Title":                  author.Name,
		"EmailSendingConfigured": emailSendingConfigured,
		"EmailFrom":              a.sender.From(),
		"WordsPerMinute":         a.config.WordsPerMinute,
		"URL":                    view.URL(c),
		"SortURL":                view.SortURL(c),
		"SortBy":                 c.Query("sort-by"),
	}

	if c.Get("hx-request") == "true" {
		if err = c.Render("partials/docs-list", templateVars); err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
		return nil
	}

	if err = c.Render("author/results", templateVars, "layout"); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	return nil
}

func (d *Controller) parseSortBy(c *fiber.Ctx) []string {
	if c.Query("sort-by") != "" {
		switch c.Query("sort-by") {
		case "pub-date-older-first":
			return []string{"Publication.Date"}
		case "pub-date-newer-first":
			return []string{"-Publication.Date"}
		}
	}
	return []string{"Series", "SeriesIndex"}
}
