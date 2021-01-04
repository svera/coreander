package webserver

import (
	"math"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html"
	"github.com/svera/coreander/config"
	"github.com/svera/coreander/indexer"
)

const (
	resultsPerPage    = 10
	maxPagesNavigator = 5
)

func Start(idx *indexer.BleveIndexer, cfg config.Config) {
	engine := html.New("./views", ".html").Reload(true).Debug(true)
	app := fiber.New(fiber.Config{
		Views: engine,
	})

	app.Get("/", func(c *fiber.Ctx) error {
		keywords := c.Query("search")
		page, err := strconv.Atoi(c.Query("page"))
		if err != nil {
			page = 1
		}
		if page < 1 {
			page = 1
		}
		if keywords != "" {
			searchResults, err := idx.Search(keywords, page, resultsPerPage)
			if err != nil {
				return fiber.ErrInternalServerError
			}
			pages := int(math.Ceil(float64(searchResults.Total) / float64(resultsPerPage)))
			return c.Render("results", fiber.Map{
				"Keywords":  keywords,
				"Results":   searchResults.Hits,
				"Total":     searchResults.Total,
				"Paginator": pagination(maxPagesNavigator, pages, page, keywords),
			}, "layout")
		}
		return c.Render("index", fiber.Map{}, "layout")
	})

	app.Static("/files", cfg.LibraryPath)
	app.Static("/css", "../css")
	app.Listen(cfg.Port)
}
