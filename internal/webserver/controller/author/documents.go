package author

import (
	"log"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/result"
	"github.com/svera/coreander/v5/internal/webserver/controller/search"
	"github.com/svera/coreander/v5/internal/webserver/model"
	"github.com/svera/coreander/v5/internal/webserver/view"
)

func (a *Controller) Documents(c fiber.Ctx) error {
	var session model.Session
	if val, ok := c.Locals("Session").(model.Session); ok {
		session = val
	}

	if session.WordsPerMinute > 0 {
		a.config.WordsPerMinute = session.WordsPerMinute
	}

	var documentResults result.Paginated[[]index.Document]
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

	searchFields, err := search.ParseDocumentSearchQuery(c, a.config.WordsPerMinute)
	if err != nil {
		log.Println(err)
		return fiber.ErrBadRequest
	}
	searchFields.AuthorSlug = authorSlug
	// ParseDocumentSearchQuery already maps an explicit sort-by query param to
	// the right bleve sort key; only its no-sort-by-given default (relevance,
	// meaningless for a plain author listing) needs overriding here.
	if c.Query("sort-by") == "" {
		searchFields.SortBy = []string{"Publication.Date"}
	}

	if documentResults, err = a.idx.SearchByAuthor(searchFields, page, model.ResultsPerPage); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	searchResults := model.AugmentedDocumentsFromDocuments(documentResults)
	if session.ID > 0 {
		searchResults = a.readingRepository.CompletedPaginatedResult(int(session.ID), searchResults)
		searchResults = a.hlRepository.HighlightedPaginatedResult(int(session.ID), searchResults)
	}

	templateVars := fiber.Map{
		"Author":               author,
		"ImageVersion":         a.getImageVersion(author.Slug),
		"DocumentSearchFields": searchFields,
		"Results":              searchResults,
		"Paginator":            view.Pagination(model.MaxPagesNavigator, searchResults, c.Queries()),
		"Title":                author.Name,
		"EmailFrom":            a.sender.From(),
		"WordsPerMinute":       a.config.WordsPerMinute,
		"URL":                  view.URL(c),
		"SortURL":              view.BaseURLWithout(c, "sort-by", "page"),
		"SortBy":               c.Query("sort-by"),
		"AdditionalSortOptions": []struct {
			Key   string
			Value string
		}{
			{"pub-date-older-first", "older"},
			{"pub-date-newer-first", "newer"},
			{"est-read-time-shorter-first", "shorter"},
			{"est-read-time-longer-first", "longer"},
		},
	}

	if c.Get("hx-request") == "true" {
		if err = c.Render("partials/author-docs-list-fragments", templateVars); err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
		return nil
	}

	if err = c.Render("author/list", templateVars, "layout"); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	return nil
}
