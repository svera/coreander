package webserver

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	fibertpl "github.com/gofiber/template/html"
	"github.com/svera/coreander/internal/i18n"
	"github.com/svera/coreander/internal/index"
	"github.com/svera/coreander/internal/infrastructure"
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

type sendAttachentFormData struct {
	File  string `form:"file"`
	Email string `form:"email"`
}

// New builds a new Fiber application and set up the required routes
func New(idx index.Reader, libraryPath, homeDir, version string, metadataReaders map[string]metadata.Reader, coverMaxWidth int, sender Sender) *fiber.App {
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

	jsFS, err := fs.Sub(embedded, "embedded/js")
	if err != nil {
		log.Fatal(err)
	}
	app.Use("/js", filesystem.New(filesystem.Config{
		Root: http.FS(jsFS),
	}))

	app.Get("/covers/:filename", func(c *fiber.Ctx) error {
		return routeCovers(c, homeDir, libraryPath, metadataReaders, coverMaxWidth)
	})

	app.Post("/send", func(c *fiber.Ctx) error {
		data := new(sendAttachentFormData)

		if err := c.BodyParser(data); err != nil {
			return err
		}

		routeSend(c, libraryPath, data.File, data.Email, sender)
		return nil
	})

	app.Get("/:lang", func(c *fiber.Ctx) error {
		emailSendingConfigured := true
		if _, ok := sender.(*infrastructure.NoEmail); ok {
			emailSendingConfigured = false
		}
		return routeSearch(c, idx, version, emailSendingConfigured)
	})

	app.Get("/", func(c *fiber.Ctx) error {
		return routeRoot(c)
	})

	app.Static("/files", libraryPath)

	return app
}

func initTemplateEngine() (*fibertpl.Engine, error) {
	cat, err := i18n.NewCatalogFromFolder(embedded, "en")
	if err != nil {
		return nil, err
	}

	message.DefaultCatalog = cat

	printers := map[string]*message.Printer{
		"es": message.NewPrinter(language.Spanish),
	}
	viewsFS, err := fs.Sub(embedded, "embedded/views")
	if err != nil {
		return nil, err
	}

	engine := fibertpl.NewFileSystem(http.FS(viewsFS), ".html")

	engine.AddFunc("t", func(lang, key string, values ...interface{}) template.HTML {
		return template.HTML(printers[lang].Sprintf(key, values...))
	})

	engine.AddFunc("dict", func(values ...interface{}) map[string]interface{} {
		if len(values)%2 != 0 {
			fmt.Println("invalid dict call")
			return nil
		}
		dict := make(map[string]interface{}, len(values)/2)
		for i := 0; i < len(values); i += 2 {
			key, ok := values[i].(string)
			if !ok {
				fmt.Println("dict keys must be strings")
				return nil
			}
			dict[key] = values[i+1]
		}
		return dict
	})

	return engine, nil
}
