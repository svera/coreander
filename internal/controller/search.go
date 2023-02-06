package controller

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/internal/metadata"
)

const (
	resultsPerPage    = 10
	maxPagesNavigator = 5
)

// Result holds the result of a search request, as well as some related metadata
type Result struct {
	Page       int
	TotalPages int
	Hits       []metadata.Metadata
	TotalHits  int
}

// Reader defines a set of reading operations over an index
type Reader interface {
	Search(keywords string, page, resultsPerPage int) (*Result, error)
	Count() (uint64, error)
	Close() error
}

func Search(c *fiber.Ctx, idx Reader, version string, emailSendingConfigured bool) error {
	lang := c.Params("lang")

	if lang != "es" && lang != "en" {
		return fiber.ErrNotFound
	}

	page, err := strconv.Atoi(c.Query("page"))
	if err != nil {
		page = 1
	}

	name := ""
	claims, err := getJWTClaimsFromCookie(c)
	if err == nil {
		name = claims["name"].(string)
	}

	var keywords string
	var searchResults *Result

	keywords = c.Query("search")
	if keywords != "" {
		searchResults, err = idx.Search(keywords, page, resultsPerPage)
		if err != nil {
			return fiber.ErrInternalServerError
		}

		return c.Render("results", fiber.Map{
			"Lang":                   lang,
			"Keywords":               keywords,
			"Results":                searchResults.Hits,
			"Total":                  searchResults.TotalHits,
			"Paginator":              pagination(maxPagesNavigator, searchResults.TotalPages, searchResults.Page, "search", keywords),
			"Title":                  "Search results",
			"Version":                version,
			"EmailSendingConfigured": emailSendingConfigured,
			"Name":                   name,
		}, "layout")
	}
	count, err := idx.Count()
	if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.Render("index", fiber.Map{
		"Lang":    lang,
		"Count":   count,
		"Title":   "Coreander",
		"Version": version,
		"Name":    name,
	}, "layout")
}
