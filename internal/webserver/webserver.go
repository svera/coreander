package webserver

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	template "github.com/gofiber/template/html"
	"github.com/svera/coreander/i18n"
	"github.com/svera/coreander/internal/index"
	"github.com/svera/coreander/internal/metadata"
	"golang.org/x/text/message"
)

const (
	resultsPerPage    = 10
	maxPagesNavigator = 5
)

// New builds a new Fiber application and set up the required routes
func New(idx index.Reader, libraryPath, homeDir string, metadataReaders map[string]metadata.Reader) *fiber.App {
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
		return coversRoute(c, libraryPath, homeDir, metadataReaders)
	})

	app.Get("/:lang", func(c *fiber.Ctx) error {
		return indexRoute(c, idx, printer)
	})

	app.Get("/", func(c *fiber.Ctx) error {
		return rootRoute(c)
	})

	app.Static("/files", libraryPath)
	dir, _ := os.Getwd()
	app.Static("/css", dir+"/public/css")

	return app
}
