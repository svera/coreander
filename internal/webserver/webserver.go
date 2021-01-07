package webserver

import (
	"fmt"
	"os"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html"
	"github.com/svera/coreander/internal/index"
)

const (
	resultsPerPage    = 10
	maxPagesNavigator = 5
)

func Start(idx index.Reader, libraryPath, port string) {
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
			return c.Render("results", fiber.Map{
				"Keywords":  keywords,
				"Results":   searchResults.Hits,
				"Total":     searchResults.TotalHits,
				"Paginator": pagination(maxPagesNavigator, searchResults.TotalPages, searchResults.Page, keywords),
			}, "layout")
		}
		count, err := idx.Count()
		if err != nil {
			return fiber.ErrInternalServerError
		}
		return c.Render("index", fiber.Map{"Count": count}, "layout")
	})

	app.Static("/files", libraryPath)
	dir, _ := os.Getwd()
	app.Static("/css", dir+"/public/css")
	app.Listen(fmt.Sprintf(":%s", port))
}
