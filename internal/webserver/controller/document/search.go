package document

import (
	"log"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/rickb777/date/v2"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/result"
	"github.com/svera/coreander/v4/internal/webserver/model"
	"github.com/svera/coreander/v4/internal/webserver/view"
)

func (d *Controller) Search(c *fiber.Ctx) error {
	var session model.Session
	if val, ok := c.Locals("Session").(model.Session); ok {
		session = val
	}

	if session.WordsPerMinute > 0 {
		d.config.WordsPerMinute = session.WordsPerMinute
	}

	var documentResults result.Paginated[[]index.Document]
	searchFields, err := d.parseSearchQuery(c)
	if err != nil {
		log.Println(err)
		return fiber.ErrBadRequest
	}

	page, err := strconv.Atoi(c.Query("page"))
	if err != nil {
		page = 1
	}

	if documentResults, err = d.idx.Search(searchFields, page, model.ResultsPerPage); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	searchResults := model.AugmentedDocumentsFromDocuments(documentResults)
	if session.ID > 0 {
		searchResults = d.readingRepository.CompletedPaginatedResult(int(session.ID), searchResults)
		searchResults = d.hlRepository.HighlightedPaginatedResult(int(session.ID), searchResults)
	}

	templateVars := fiber.Map{
		"SearchFields":       searchFields,
		"Results":            searchResults,
		"Paginator":          view.Pagination(model.MaxPagesNavigator, searchResults, c.Queries()),
		"Title":              "Search results",
		"DocumentsSearchPage": true,
		"EmailFrom":          d.sender.From(),
		"WordsPerMinute":          d.config.WordsPerMinute,
		"URL":                     view.URL(c),
		"SortURL":                 view.BaseURLWithout(c, "sort-by", "page"),
		"SortBy":                  c.Query("sort-by"),
		"AvailableLanguages":      c.Locals("AvailableLanguages"),
		"AdditionalSortOptions": []struct {
			Key   string
			Value string
		}{
			{"relevance", "relevance"},
			{"pub-date-older-first", "older"},
			{"pub-date-newer-first", "newer"},
			{"est-read-time-shorter-first", "shorter"},
			{"est-read-time-longer-first", "longer"},
		},
	}

	if c.Get("hx-request") == "true" {
		if err = c.Render("partials/docs-list-fragments", templateVars); err != nil {
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
		Keywords:        c.Query("search"),
		Language:        c.Query("language"),
		Subjects:        c.Query("subjects"),
		SortBy:          d.parseSortBy(c),
		EstReadTimeFrom: c.QueryFloat("est-read-time-from", 0),
		EstReadTimeTo:   c.QueryFloat("est-read-time-to", 0),
		WordsPerMinute:  d.config.WordsPerMinute,
	}

	if c.Query("pub-date-from") != "" {
		pubDateFrom, err := date.ParseISO(c.Query("pub-date-from"))
		if err != nil {
			return searchFields, err
		}
		searchFields.PubDateFrom = pubDateFrom
	}

	if c.Query("pub-date-to") != "" {
		pubDateTo, err := date.ParseISO(c.Query("pub-date-to"))
		if err != nil {
			return searchFields, err
		}
		searchFields.PubDateTo = pubDateTo
	}

	if searchFields.PubDateTo != 0 && searchFields.PubDateFrom > searchFields.PubDateTo {
		searchFields.PubDateFrom, searchFields.PubDateTo = searchFields.PubDateTo, searchFields.PubDateFrom
	}

	if searchFields.EstReadTimeTo != 0 && searchFields.EstReadTimeFrom > searchFields.EstReadTimeTo {
		searchFields.EstReadTimeFrom, searchFields.EstReadTimeTo = searchFields.EstReadTimeTo, searchFields.EstReadTimeFrom
	}

	return searchFields, nil
}

func (d *Controller) parseSortBy(c *fiber.Ctx) []string {
	if c.Query("sort-by") != "" {
		switch c.Query("sort-by") {
		case "pub-date-older-first":
			return []string{"Publication.Date"}
		case "pub-date-newer-first":
			return []string{"-Publication.Date"}
		case "est-read-time-shorter-first":
			return []string{"Words"}
		case "est-read-time-longer-first":
			return []string{"-Words"}
		}
	}
	return []string{"-_score", "Series", "SeriesIndex"}
}

// Subjects returns all unique subjects from the index as JSON
func (d *Controller) Subjects(c *fiber.Ctx) error {
	subjects, err := d.idx.Subjects()
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	return c.JSON(subjects)
}
