package webserver

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"sort"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cache"
	"github.com/gofiber/fiber/v2/middleware/favicon"
	"github.com/svera/coreander/v3/internal/i18n"
	"github.com/svera/coreander/v3/internal/index"
	"github.com/svera/coreander/v3/internal/webserver/infrastructure"
	"golang.org/x/exp/slices"
	"golang.org/x/text/message"
)

var (
	//go:embed embedded
	embedded           embed.FS
	cssFS              fs.FS
	jsFS               fs.FS
	imagesFS           fs.FS
	printers           map[string]*message.Printer
	supportedLanguages []string
)

const (
	defaultHttpPort  = 80
	defaultHttpsPort = 443
)

type Config struct {
	Version               string
	SessionTimeout        time.Duration
	MinPasswordLength     int
	WordsPerMinute        float64
	JwtSecret             []byte
	Hostname              string
	Port                  int
	HomeDir               string
	LibraryPath           string
	CoverMaxWidth         int
	RequireAuth           bool
	UploadDocumentMaxSize int
}

type Sender interface {
	Send(address, subject, body string) error
	SendDocument(address string, libraryPath string, fileName string) error
	From() string
}

type ProgressInfo interface {
	IndexingProgress() (index.Progress, error)
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

	supportedLanguages = getSupportedLanguages()
}

// New builds a new Fiber application and set up the required routes
func New(cfg Config, controllers Controllers, sender Sender, progress ProgressInfo) *fiber.App {
	viewsFS, err := fs.Sub(embedded, "embedded/views")
	if err != nil {
		log.Fatal(err)
	}

	engine, err := infrastructure.TemplateEngine(viewsFS, printers)
	if err != nil {
		log.Fatal(err)
	}

	app := fiber.New(fiber.Config{
		Views:                        engine,
		DisableStartupMessage:        true,
		AppName:                      cfg.Version,
		PassLocalsToViews:            true,
		ErrorHandler:                 errorHandler,
		BodyLimit:                    cfg.UploadDocumentMaxSize * 1024 * 1024,
		DisablePreParseMultipartForm: true,
		StreamRequestBody:            true,
	})

	app.Use(SetFQDN(cfg))

	app.Use(SetProgress(progress))

	app.Use(favicon.New())

	app.Use(cache.New(cache.Config{
		ExpirationGenerator: func(c *fiber.Ctx, cfg *cache.Config) time.Duration {
			newCacheTime, _ := strconv.Atoi(c.GetRespHeader("Cache-Time", "0"))
			return time.Second * time.Duration(newCacheTime)
		},
	}),
	)

	routes(app, controllers, cfg.JwtSecret, sender, cfg.RequireAuth)
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

func chooseBestLanguage(c *fiber.Ctx) string {
	lang := c.Params("lang")
	if !slices.Contains(supportedLanguages, lang) {
		lang = c.AcceptsLanguages(supportedLanguages...)
		if lang == "" {
			lang = "en"
		}
	}

	return lang
}

func urlPort(protocol string, port int) string {
	urlPort := fmt.Sprintf(":%d", port)
	if (port == defaultHttpPort && protocol == "http") ||
		(port == defaultHttpsPort && protocol == "https") {
		urlPort = ""
	}
	return urlPort
}

func errorHandler(c *fiber.Ctx, err error) error {
	// Status code defaults to 500
	code := fiber.StatusInternalServerError
	// Retrieve the custom status code if it's a *fiber.Error
	var e *fiber.Error
	if errors.As(err, &e) {
		code = e.Code
	}

	// Send custom error page
	err = c.Status(code).Render(
		fmt.Sprintf("errors/%d", code),
		fiber.Map{
			"Lang":    chooseBestLanguage(c),
			"Title":   "Coreander",
			"Version": c.App().Config().AppName,
		},
		"layout")

	if err != nil {
		log.Println(err)
		// In case the Render fails
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	return nil
}
