package webserver

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/internal/index"
)

func routeSearch(c *fiber.Ctx, idx index.Reader, version string) error {
	lang := c.Params("lang")

	page, err := strconv.Atoi(c.Query("page"))
	if err != nil {
		page = 1
	}

	var keywords string
	var searchResults *index.Result

	keywords = c.Query("search")
	if keywords != "" {
		searchResults, err = idx.Search(keywords, page, resultsPerPage)
		if err != nil {
			return fiber.ErrInternalServerError
		}

		return c.Render("results", fiber.Map{
			"Lang":      lang,
			"Keywords":  keywords,
			"Results":   searchResults.Hits,
			"Total":     searchResults.TotalHits,
			"Paginator": pagination(maxPagesNavigator, searchResults.TotalPages, searchResults.Page, "search", keywords),
			"Title":     "search_results",
			"Version":   version,
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
	}, "layout")
}
