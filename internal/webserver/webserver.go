package webserver

import (
	"embed"
	"io/fs"
	"log"
	"sort"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cache"
	"github.com/gofiber/fiber/v2/middleware/favicon"
	"github.com/svera/coreander/v3/internal/i18n"
	"github.com/svera/coreander/v3/internal/infrastructure"
	"golang.org/x/exp/slices"
	"golang.org/x/text/message"
)

var (
	//go:embed embedded
	embedded embed.FS
	cssFS    fs.FS
	jsFS     fs.FS
	imagesFS fs.FS
	printers map[string]*message.Printer
)

type Config struct {
	Version           string
	SessionTimeout    time.Duration
	MinPasswordLength int
	WordsPerMinute    float64
	JwtSecret         []byte
	Hostname          string
	Port              int
	HomeDir           string
	LibraryPath       string
	CoverMaxWidth     int
	RequireAuth       bool
}

type Sender interface {
	Send(address, subject, body string) error
	SendDocument(address string, libraryPath string, fileName string) error
	From() string
}

func init() {
	var err error

	cssFS, err = fs.Sub(embedded, "embedded/css")
	if err != nil {
		log.Fatal(err)
	}

	jsFS, err = fs.Sub(embedded, "embedded/js")
	if err != nil {
		log.Fatal(err)
	}

	imagesFS, err = fs.Sub(embedded, "embedded/images")
	if err != nil {
		log.Fatal(err)
	}

	dir, err := fs.Sub(embedded, "embedded/translations")
	if err != nil {
		log.Fatal(err)
	}

	printers, err = i18n.Printers(dir, "en")
	if err != nil {
		log.Fatal(err)
	}
}

// New builds a new Fiber application and set up the required routes
func New(cfg Config, controllers Controllers) *fiber.App {
	viewsFS, err := fs.Sub(embedded, "embedded/views")
	if err != nil {
		log.Fatal(err)
	}

	engine, err := infrastructure.TemplateEngine(viewsFS, printers)
	if err != nil {
		log.Fatal(err)
	}

	app := fiber.New(fiber.Config{
		Views:                 engine,
		DisableStartupMessage: true,
		AppName:               cfg.Version,
		PassLocalsToViews:     true,
		ErrorHandler:          controllers.ErrorHandler,
	})

	app.Use(favicon.New())

	app.Use(cache.New(cache.Config{
		ExpirationGenerator: func(c *fiber.Ctx, cfg *cache.Config) time.Duration {
			newCacheTime, _ := strconv.Atoi(c.GetRespHeader("Cache-Time", "0"))
			return time.Second * time.Duration(newCacheTime)
		},
	}),
	)

	routes(app, controllers, getSupportedLanguages())
	return app
}

func getSupportedLanguages() []string {
	langs := make([]string, len(printers))

	i := 0
	for k := range printers {
		langs[i] = k
		i++
	}

	sort.Strings(langs)
	return langs
}

func chooseBestLanguage(c *fiber.Ctx, supportedLanguages []string) string {
	lang := c.Params("lang")
	if !slices.Contains(supportedLanguages, lang) {
		lang = c.AcceptsLanguages(supportedLanguages...)
		if lang == "" {
			lang = "en"
		}
	}

	return lang
}
