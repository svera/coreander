package webserver

import (
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/redirect/v2"
	"github.com/gofiber/template/html"
	"github.com/svera/coreander/i18n"
	"github.com/svera/coreander/internal/index"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

const (
	resultsPerPage    = 10
	maxPagesNavigator = 5
)

// New builds a new Fiber application and set up the required routes
func New(idx index.Reader, libraryPath string) *fiber.App {
	cat, err := i18n.NewCatalogFromFolder("./translations", "en")
	if err != nil {
		log.Fatal(err)
	}

	message.DefaultCatalog = cat

	var printer *message.Printer
	engine := html.New("./views", ".html").Reload(true)
	engine.AddFunc("t", func(key string, values ...interface{}) string {
		return printer.Sprintf(key, values...)
	})

	app := fiber.New(fiber.Config{
		Views: engine,
	})

	app.Use(redirect.New(redirect.Config{
		Rules: map[string]string{
			"/": "/en",
		},
		StatusCode: http.StatusMovedPermanently,
	}))

	app.Get("/:lang", func(c *fiber.Ctx) error {
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
	})

	app.Static("/files", libraryPath)
	dir, _ := os.Getwd()
	app.Static("/css", dir+"/public/css")

	return app
}
