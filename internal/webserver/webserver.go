package webserver

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cache"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/favicon"
	"github.com/svera/coreander/v4/internal/i18n"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"github.com/svera/coreander/v4/internal/webserver/model"
	"golang.org/x/exp/slices"
)

var (
	//go:embed embedded
	embedded           embed.FS
	cssFS              fs.FS
	jsFS               fs.FS
	imagesFS           fs.FS
	translator         i18n.Translator
	supportedLanguages []string
)

type Config struct {
	Version                    string
	SessionTimeout             time.Duration
	RecoveryTimeout            time.Duration
	InvitationTimeout          time.Duration
	MinPasswordLength          int
	WordsPerMinute             float64
	JwtSecret                  []byte
	Hostname                   string
	FQDN                       string
	Port                       int
	HomeDir                    string
	CacheDir                   string
	LibraryPath                string
	AuthorImageMaxWidth        int
	CoverMaxWidth              int
	RequireAuth                bool
	UploadDocumentMaxSize      int
	ClientStaticCacheTTL       int
	ClientDynamicImageCacheTTL int
	ServerStaticCacheTTL       int
	ServerDynamicImageCacheTTL int
	ShareCommentMaxSize        int
	ShareMaxRecipients         int
}

type Sender interface {
	Send(address, subject, body string) error
	SendBCC(addresses []string, subject, body string) error
	SendDocument(address, subject, libraryPath, fileName string) error
	From() string
}

type ProgressInfo interface {
	IndexingProgress() (index.Progress, error)
	Languages() ([]string, error)
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

	translator, err = i18n.New(dir, "en")
	if err != nil {
		log.Fatal(err)
	}

	supportedLanguages = translator.SupportedLanguages()
}

// getSupportedLanguages returns the list of supported languages
func getSupportedLanguages() []string {
	return supportedLanguages
}

// New builds a new Fiber application and set up the required routes
func New(cfg Config, controllers Controllers, sender Sender, idx ProgressInfo, usersRepository *model.UserRepository) *fiber.App {
	viewsFS, err := fs.Sub(embedded, "embedded/views")
	if err != nil {
		log.Fatal(err)
	}

	engine, err := infrastructure.TemplateEngine(viewsFS, translator)
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

	app.Use(
		SetFQDN(cfg),
		SetProgress(idx),
		favicon.New(),
		cache.New(cache.Config{
			ExpirationGenerator: func(c *fiber.Ctx, cfg *cache.Config) time.Duration {
				newCacheTime, _ := strconv.Atoi(c.GetRespHeader("Cache-Time", "0"))
				return time.Second * time.Duration(newCacheTime)
			},
		}),
		OneTimeMessages(),
		compress.New(),
	)

	routes(app, controllers, cfg.JwtSecret, sender, translator, cfg, idx, usersRepository)
	return app
}

func chooseBestLanguage(c *fiber.Ctx) string {
	lang := c.Query("l")
	if lang != "" {
		c.Cookie(&fiber.Cookie{
			Name:     "locale",
			Value:    lang,
			Path:     "/",
			MaxAge:   34560000, // 400 days which is the life limit imposed by Chrome
			Secure:   false,
			HTTPOnly: true,
		})
		return lang
	}
	lang = c.Cookies("locale")
	if !slices.Contains(supportedLanguages, lang) {
		lang = c.AcceptsLanguages(supportedLanguages...)
		if lang == "" {
			lang = "en"
		}
	}

	return lang
}

func errorHandler(c *fiber.Ctx, err error) error {
	// Status code defaults to 500
	code := fiber.StatusInternalServerError
	// Retrieve the custom status code if it's a *fiber.Error
	var e *fiber.Error
	if errors.As(err, &e) {
		code = e.Code
	}

	session, _ := c.Locals("Session").(model.Session)
	// Send custom error page
	c.Status(code)

	// Only render the error page if the request is not an htmx request
	if c.Get("hx-request") == "true" {
		return nil
	}

	err = c.Render(
		fmt.Sprintf("errors/%d", code),
		fiber.Map{
			"Lang":    chooseBestLanguage(c),
			"Title":   "Error",
			"Session": session,
		},
		"layout")

	if err != nil {
		log.Println(err)
		// In case the Render fails
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	return nil
}
