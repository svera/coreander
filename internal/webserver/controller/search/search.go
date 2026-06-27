package search

import (
	"log"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v5/internal/index"
"github.com/svera/coreander/v5/internal/webserver/model"
	"github.com/svera/coreander/v5/internal/webserver/view"
)

func (s *Controller) SearchDocuments(c fiber.Ctx) error {
	c.Locals("SearchType", TypeDocuments)
	return s.Search(c)
}

func (s *Controller) SearchAuthors(c fiber.Ctx) error {
	c.Locals("SearchType", TypeAuthors)
	return s.Search(c)
}

func (s *Controller) Search(c fiber.Ctx) error {
	searchType := searchTypeFromContext(c)

	var session model.Session
	if val, ok := c.Locals("Session").(model.Session); ok {
		session = val
	}

	wordsPerMinute := s.config.WordsPerMinute
	if session.WordsPerMinute > 0 {
		wordsPerMinute = session.WordsPerMinute
	}

	page := parsePage(c)

	if searchType == TypeAuthors {
		return s.renderAuthorSearch(c, session, page)
	}
	return s.renderDocumentSearch(c, session, page, wordsPerMinute)
}

func (s *Controller) renderDocumentSearch(c fiber.Ctx, session model.Session, page int, wordsPerMinute float64) error {
	searchFields, err := parseDocumentSearchQuery(c, wordsPerMinute)
	if err != nil {
		log.Println(err)
		return fiber.ErrBadRequest
	}

	documentResults, err := s.idx.Search(searchFields, page, model.ResultsPerPage)
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	searchResults := model.AugmentedDocumentsFromDocuments(documentResults)
	if session.ID > 0 {
		searchResults = s.readingRepository.CompletedPaginatedResult(int(session.ID), searchResults)
		searchResults = s.hlRepository.HighlightedPaginatedResult(int(session.ID), searchResults)
	}

	authorSearchFields, err := parseAuthorSearchQuery(c)
	if err != nil {
		log.Println(err)
		return fiber.ErrBadRequest
	}

	docCount, authorCount, err := s.tabCounts(searchFields, authorSearchFields)
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	templateVars := s.baseTemplateVars(c, TypeDocuments)
	templateVars["SearchFields"] = searchFields
	templateVars["DocumentSearchFields"] = searchFields
	templateVars["SearchQuery"] = searchFields.Keywords
	templateVars["Results"] = searchResults
	templateVars["Paginator"] = view.Pagination(model.MaxPagesNavigator, searchResults, c.Queries())
	templateVars["Title"] = "Search results"
	templateVars["WordsPerMinute"] = wordsPerMinute
	templateVars["AdditionalSortOptions"] = documentSortOptions()
	templateVars["DocumentsTotalHits"] = docCount
	templateVars["AuthorsTotalHits"] = authorCount

	return s.renderSearch(c, templateVars, "partials/docs-list-fragments")
}

func (s *Controller) renderAuthorSearch(c fiber.Ctx, session model.Session, page int) error {
	searchFields, err := parseAuthorSearchQuery(c)
	if err != nil {
		log.Println(err)
		return fiber.ErrBadRequest
	}

	authorResults, err := s.idx.SearchAuthors(searchFields, page, int(model.ResultsPerPage))
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	keywords := searchFields.Name
	documentSearchFields, err := parseDocumentSearchQuery(c, 0)
	if err != nil {
		log.Println(err)
		return fiber.ErrBadRequest
	}

	docCount, authorCount, err := s.tabCounts(documentSearchFields, searchFields)
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	templateVars := s.baseTemplateVars(c, TypeAuthors)
	templateVars["SearchFields"] = searchFields
	templateVars["AuthorSearchFields"] = searchFields
	templateVars["DocumentSearchFields"] = index.SearchFields{Keywords: keywords}
	templateVars["SearchQuery"] = keywords
	templateVars["SelectedGender"] = c.Query("gender")
	templateVars["Results"] = authorResults
	templateVars["Paginator"] = view.Pagination(model.MaxPagesNavigator, authorResults, c.Queries())
	templateVars["Title"] = "Search authors"
	templateVars["AdditionalSortOptions"] = authorSortOptions()
	templateVars["DocumentsTotalHits"] = docCount
	templateVars["AuthorsTotalHits"] = authorCount

	return s.renderSearch(c, templateVars, "partials/authors-list-fragments")
}

func (s *Controller) baseTemplateVars(c fiber.Ctx, searchType string) fiber.Map {
	return fiber.Map{
		"SearchType":         searchType,
		"SearchPage":         true,
		"EmailFrom":          s.sender.From(),
		"URL":                view.URL(c),
		"SortURL":            view.BaseURLWithout(c, "sort-by", "page"),
		"SortBy":             c.Query("sort-by"),
		"AuthorSearchFields": index.AuthorSearchFields{},
		"DocumentSearchFields": index.SearchFields{},
	}
}

func (s *Controller) renderSearch(c fiber.Ctx, templateVars fiber.Map, fragmentTemplate string) error {
	if c.Get("hx-request") == "true" {
		if err := c.Render(fragmentTemplate, templateVars); err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
		return nil
	}

	if err := c.Render("search/list", templateVars, "layout"); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	return nil
}

func (s *Controller) tabCounts(docFields index.SearchFields, authorFields index.AuthorSearchFields) (docCount, authorCount int, err error) {
	docCount, err = s.idx.CountDocuments(docFields)
	if err != nil {
		return 0, 0, err
	}
	authorCount, err = s.idx.CountAuthors(authorFields)
	if err != nil {
		return 0, 0, err
	}
	return docCount, authorCount, nil
}

func documentSortOptions() []struct {
	Key   string
	Value string
} {
	return []struct {
		Key   string
		Value string
	}{
		{"relevance", "relevance"},
		{"pub-date-older-first", "older"},
		{"pub-date-newer-first", "newer"},
		{"est-read-time-shorter-first", "shorter"},
		{"est-read-time-longer-first", "longer"},
	}
}

func authorSortOptions() []struct {
	Key   string
	Value string
} {
	return []struct {
		Key   string
		Value string
	}{
		{"name-a-z", "name A-Z"},
		{"name-z-a", "name Z-A"},
		{"birth-older-first", "birth older first"},
		{"birth-newer-first", "birth newer first"},
		{"death-older-first", "death older first"},
		{"death-newer-first", "death newer first"},
		{"documents-more-first", "documents more first"},
		{"documents-fewer-first", "documents fewer first"},
	}
}
