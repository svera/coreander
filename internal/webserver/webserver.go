package webserver

import (
	"embed"
	"html/template"
	"io/fs"
	"log"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	jwtware "github.com/gofiber/jwt/v2"
	fibertpl "github.com/gofiber/template/html"
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

var languages = map[string]struct{}{"en": {}, "es": {}}

//go:embed embedded
var embedded embed.FS

// New builds a new Fiber application and set up the required routes
func New(idx index.Reader, libraryPath, homeDir, version, secret string, metadataReaders map[string]metadata.Reader, coverMaxWidth int) *fiber.App {
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

	if secret != "" {
		app.Get("/:lang/login", func(c *fiber.Ctx) error {
			return routeLogInForm(c, version)
		})

		app.Post("/:lang/login", func(c *fiber.Ctx) error {
			return routeLogIn(c, secret)
		})
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

	app.Get("/", func(c *fiber.Ctx) error {
		return routeRoot(c)
	})

	// JWT Middleware
	if secret != "" {
		app.Use("/:lang", jwtware.New(jwtware.Config{
			SigningKey:  []byte(secret),
			TokenLookup: "cookie:coreander",
			ErrorHandler: func(c *fiber.Ctx, err error) error {
				lang := c.Params("lang")
				if _, ok := languages[lang]; !ok {
					lang = getBaseLanguage(c)
				}
				return c.Redirect(lang + "/login")
			},
		}))
	}

	app.Get("/covers/:filename", func(c *fiber.Ctx) error {
		return routeCovers(c, homeDir, libraryPath, metadataReaders, coverMaxWidth)
	})

	app.Static("/files", libraryPath)

	app.Get("/:lang", func(c *fiber.Ctx) error {
		return routeSearch(c, idx, version)
	})

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
		"en": message.NewPrinter(language.English),
	}
	viewsFS, err := fs.Sub(embedded, "embedded/views")
	if err != nil {
		return nil, err
	}

	engine := fibertpl.NewFileSystem(http.FS(viewsFS), ".html")
	engine.AddFunc("t", func(lang, key string, values ...interface{}) template.HTML {
		return template.HTML(printers[lang].Sprintf(key, values...))
	})

	return engine, nil
}

func getBaseLanguage(c *fiber.Ctx) string {
	acceptHeader := c.Get(fiber.HeaderAcceptLanguage)
	languageMatcher := language.NewMatcher([]language.Tag{
		language.English,
		language.Spanish,
	})

	t, _, _ := language.ParseAcceptLanguage(acceptHeader)
	tag, _, _ := languageMatcher.Match(t...)
	baseLang, _ := tag.Base()
	return baseLang.String()
}
