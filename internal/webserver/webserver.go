package webserver

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/gofiber/fiber/v2"
	template "github.com/gofiber/template/html"
	"github.com/svera/coreander/i18n"
	"github.com/svera/coreander/internal/index"
	"github.com/svera/coreander/internal/metadata"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

const (
	resultsPerPage    = 10
	maxPagesNavigator = 5
)

// New builds a new Fiber application and set up the required routes
func New(idx index.Reader, libraryPath, homeDir string) *fiber.App {
	cat, err := i18n.NewCatalogFromFolder("./translations", "en")
	if err != nil {
		log.Fatal(err)
	}

	message.DefaultCatalog = cat

	var printer *message.Printer
	engine := template.New("./views", ".html").Reload(true)
	engine.AddFunc("t", func(key string, values ...interface{}) string {
		return printer.Sprintf(key, values...)
	})

	app := fiber.New(fiber.Config{
		Views: engine,
	})

	app.Get("/covers/:filename", func(c *fiber.Ctx) error {
		fileName, err := url.QueryUnescape(c.Params("filename"))
		if err != nil {
			return err
		}
		info, err := os.Stat(fmt.Sprintf("%s/coreander/cache/covers/%s.jpg", homeDir, fileName))
		if os.IsNotExist(err) {
			err = metadata.EpubCover(
				fmt.Sprintf("%s/%s", libraryPath, fileName),
				fmt.Sprintf("%s/coreander/cache/covers", homeDir),
			)
			if err != nil {
				log.Println(err)
				return fiber.ErrInternalServerError
			}
		} else if info.IsDir() {
			return fiber.ErrBadRequest
		}
		image, err := ioutil.ReadFile(fmt.Sprintf("%s/coreander/cache/covers/%s.jpg", homeDir, fileName))
		if err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
		c.Response().Header.Set(fiber.HeaderContentType, "image/jpeg")
		c.Response().BodyWriter().Write(image)
		return nil
	})

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

	app.Get("/", func(c *fiber.Ctx) error {
		acceptHeader := c.Get(fiber.HeaderAcceptLanguage)
		languageMatcher := language.NewMatcher([]language.Tag{
			language.English,
			language.Spanish,
		})

		t, _, _ := language.ParseAcceptLanguage(acceptHeader)
		tag, _, _ := languageMatcher.Match(t...)
		baseLang, _ := tag.Base()
		return c.Redirect(fmt.Sprintf("/%s", baseLang.String()))
	})

	app.Static("/files", libraryPath)
	dir, _ := os.Getwd()
	app.Static("/css", dir+"/public/css")

	return app
}
