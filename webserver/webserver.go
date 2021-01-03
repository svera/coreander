package webserver

import (
	"math"
	"strconv"

	"github.com/blevesearch/bleve"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html"
	"github.com/svera/coreander/config"
)

const (
	resultsPerPage    = 10
	maxPagesNavigator = 10
)

func Start(idx bleve.Index, cfg config.Config) {
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
			query := bleve.NewMatchQuery(keywords)
			search := bleve.NewSearchRequestOptions(query, resultsPerPage, (page-1)*resultsPerPage, false)
			search.Fields = []string{"Title", "Author", "Description"}
			searchResults, _ := idx.Search(search)
			if searchResults.Total < uint64(page-1)*10 {
				page = 1
				search = bleve.NewSearchRequestOptions(query, resultsPerPage, (page-1)*resultsPerPage, false)
				search.Fields = []string{"Title", "Author", "Description"}
				searchResults, _ = idx.Search(search)
			}
			pages := int(math.Ceil(float64(searchResults.Total) / float64(resultsPerPage)))
			idx.Search(search)
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
