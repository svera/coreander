package webserver

import (
	"embed"
	"io/fs"
	"log"
	"net/http"

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
func New(idx index.Reader, libraryPath, homeDir, version string, metadataReaders map[string]metadata.Reader, coverMaxWidth int) *fiber.App {
	engine, err := initTemplateEngine()
	if err != nil {
		log.Fatal(err)
	}

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
		return routeCovers(c, homeDir, libraryPath, metadataReaders, coverMaxWidth)
	})

	app.Get("/:lang", func(c *fiber.Ctx) error {
		return routeSearch(c, idx, version)
	})

	app.Get("/", func(c *fiber.Ctx) error {
		return routeRoot(c)
	})

	app.Static("/files", libraryPath)

	return app
}

func initTemplateEngine() (*template.Engine, error) {
	cat, err := i18n.NewCatalogFromFolder(embedded, "en")
	if err != nil {
		return nil, err
	}

	message.DefaultCatalog = cat

	printers := map[string]*message.Printer{
		"es": message.NewPrinter(language.Spanish),
		"en": message.NewPrinter(language.English),
	}
	viewsFS, err := fs.Sub(embedded, "embedded/views")
	if err != nil {
		return nil, err
	}

	engine := template.NewFileSystem(http.FS(viewsFS), ".html")
	engine.AddFunc("t", func(lang, key string, values ...interface{}) string {
		return printers[lang].Sprintf(key, values...)
	})

	return engine, nil
}
