package webserver

import (
	"embed"
	"fmt"
	"io/fs"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	template "github.com/gofiber/template/html"
	"github.com/svera/coreander/internal/i18n"
	"github.com/svera/coreander/internal/index"
	"github.com/svera/coreander/internal/metadata"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

const (
	resultsPerPage    = 10
	maxPagesNavigator = 5
)

//go:embed embedded
var embedded embed.FS

// New builds a new Fiber application and set up the required routes
func New(idx index.Reader, libraryPath, homeDir string, metadataReaders map[string]metadata.Reader) *fiber.App {
	cat, err := i18n.NewCatalogFromFolder(embedded, "en")
	if err != nil {
		log.Fatal(err)
	}

	message.DefaultCatalog = cat

	var printer *message.Printer
	viewsFS, err := fs.Sub(embedded, "embedded/views")
	if err != nil {
		log.Fatal(err)
	}

	engine := template.NewFileSystem(http.FS(viewsFS), ".html")
	engine.AddFunc("t", func(key string, values ...interface{}) string {
		return printer.Sprintf(key, values...)
	})

	app := fiber.New(fiber.Config{
		Views:                 engine,
		DisableStartupMessage: true,
	})

	cssFS, err := fs.Sub(embedded, "embedded/css")
	if err != nil {
		log.Fatal(err)
	}
	app.Use("/css", filesystem.New(filesystem.Config{
		Root: http.FS(cssFS),
	}))

	app.Get("/covers/:filename", func(c *fiber.Ctx) error {
		fileName, err := url.QueryUnescape(c.Params("filename"))
		if err != nil {
			return err
		}
		ext := filepath.Ext(fileName)
		if _, ok := metadataReaders[ext]; !ok {
			return fiber.ErrBadRequest
		}
		info, err := os.Stat(fmt.Sprintf("%s/coreander/cache/covers/%s.jpg", homeDir, fileName))
		if os.IsNotExist(err) {
			err = metadataReaders[ext].Cover(
				fmt.Sprintf("%s/%s", libraryPath, fileName),
				fmt.Sprintf("%s/coreander/cache/covers", homeDir),
			)
			if err != nil {
				log.Println(err)
				input, err := embedded.ReadFile("embedded/images/generic.jpg")
				if err != nil {
					log.Println(err)
					return fiber.ErrInternalServerError
				}

				destinationFile := fmt.Sprintf("%s/coreander/cache/covers/%s.jpg", homeDir, fileName)
				err = ioutil.WriteFile(destinationFile, input, 0644)
				if err != nil {
					log.Println("Error creating", destinationFile)
					return fiber.ErrInternalServerError
				}
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

	return app
}
