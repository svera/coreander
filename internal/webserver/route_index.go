package webserver

import (
	"net/http"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/internal/index"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

func indexRoute(c *fiber.Ctx, idx index.Reader, printer *message.Printer) error {
	lang := c.Params("lang")
	switch lang {
	case "es":
		printer = message.NewPrinter(language.Spanish)
	case "en":
		printer = message.NewPrinter(language.English)
	default:
		return c.SendStatus(http.StatusNotFound)
	}
	keywords := c.Query("search")
	page, err := strconv.Atoi(c.Query("page"))
	if err != nil {
		page = 1
	}

	if keywords != "" {
		searchResults, err := idx.Search(keywords, page, resultsPerPage)
		if err != nil {
			return fiber.ErrInternalServerError
		}
		return c.Render("results", fiber.Map{
			"Lang":      lang,
			"Keywords":  keywords,
			"Results":   searchResults.Hits,
			"Total":     searchResults.TotalHits,
			"Paginator": pagination(maxPagesNavigator, searchResults.TotalPages, searchResults.Page, keywords),
			"Title":     "Coreander -  Search results",
		}, "layout")
	}
	count, err := idx.Count()
	if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.Render("index", fiber.Map{
		"Lang":  lang,
		"Count": count,
		"Title": "Coreander",
	}, "layout")
}
