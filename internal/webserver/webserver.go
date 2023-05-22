package webserver

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cache"
	"github.com/gofiber/fiber/v2/middleware/favicon"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	jwtware "github.com/gofiber/jwt/v3"
	"github.com/svera/coreander/v2/internal/controller"
	"github.com/svera/coreander/v2/internal/infrastructure"
	"github.com/svera/coreander/v2/internal/jwtclaimsreader"
	"github.com/svera/coreander/v2/internal/metadata"
	"github.com/svera/coreander/v2/internal/model"
	"golang.org/x/exp/slices"
	"golang.org/x/text/message"
	"gorm.io/gorm"
)

var (
	//go:embed embedded
	embedded           embed.FS
	supportedLanguages []string
)

type Config struct {
	LibraryPath       string
	HomeDir           string
	Version           string
	CoverMaxWidth     int
	JwtSecret         []byte
	RequireAuth       bool
	MinPasswordLength int
	WordsPerMinute    float64
	Hostname          string
	Port              int
	SessionTimeout    time.Duration
}

type Sender interface {
	Send(address, subject, body string) error
	SendDocument(address string, libraryPath string, fileName string) error
	From() string
}

// New builds a new Fiber application and set up the required routes
func New(cfg Config, printers map[string]*message.Printer) *fiber.App {
	viewsFS, err := fs.Sub(embedded, "embedded/views")
	if err != nil {
		log.Fatal(err)
	}

	engine, err := infrastructure.TemplateEngine(viewsFS, printers)
	if err != nil {
		log.Fatal(err)
	}

	supportedLanguages = getSupportedLanguages(printers)

	app := fiber.New(fiber.Config{
		Views:                 engine,
		DisableStartupMessage: true,
		AppName:               cfg.Version,
		PassLocalsToViews:     true,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
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
					"Lang":    chooseBestLanguage(c, supportedLanguages),
					"Title":   "Coreander",
					"Session": jwtclaimsreader.SessionData(c),
					"Version": c.App().Config().AppName,
				},
				"layout")

			if err != nil {
				log.Println(err)
				// In case the Render fails
				return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
			}

			return nil
		},
	})

	app.Use(favicon.New())

	app.Use(cache.New(cache.Config{
		ExpirationGenerator: func(c *fiber.Ctx, cfg *cache.Config) time.Duration {
			newCacheTime, _ := strconv.Atoi(c.GetRespHeader("Cache-Time", "0"))
			return time.Second * time.Duration(newCacheTime)
		},
	}),
	)

	initResources(app)
	return app
}

func initResources(app *fiber.App) {
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

	imagesFS, err := fs.Sub(embedded, "embedded/images")
	if err != nil {
		log.Fatal(err)
	}
	app.Use("/images", filesystem.New(filesystem.Config{
		Root: http.FS(imagesFS),
	}))
}

func Routes(app *fiber.App, idx controller.Reader, cfg Config, metadataReaders map[string]metadata.Reader, sender Sender, db *gorm.DB, printers map[string]*message.Printer) {
	usersRepository := &model.UserRepository{DB: db}

	authCfg := controller.AuthConfig{
		MinPasswordLength: cfg.MinPasswordLength,
		Secret:            cfg.JwtSecret,
		Hostname:          cfg.Hostname,
		Port:              cfg.Port,
		SessionTimeout:    cfg.SessionTimeout,
	}

	authController := controller.NewAuth(usersRepository, sender, authCfg, printers)

	langGroup := app.Group(fmt.Sprintf("/:lang<regex(%s)>", strings.Join(supportedLanguages, "|")), func(c *fiber.Ctx) error {
		pathMinusLang := c.Path()[3:]
		query := string(c.Request().URI().QueryString())
		if query != "" {
			pathMinusLang = pathMinusLang + "?" + query
		}
		c.Locals("Lang", c.Params("lang"))
		c.Locals("SupportedLanguages", supportedLanguages)
		c.Locals("PathMinusLang", pathMinusLang)
		return c.Next()
	})

	allowIfNotLoggedInMiddleware := jwtware.New(jwtware.Config{
		SigningKey:    cfg.JwtSecret,
		SigningMethod: "HS256",
		TokenLookup:   "cookie:coreander",
		SuccessHandler: func(c *fiber.Ctx) error {
			return fiber.ErrForbidden
		},
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Next()
		},
	})

	langGroup.Get("/login", allowIfNotLoggedInMiddleware, authController.Login)
	langGroup.Post("login", allowIfNotLoggedInMiddleware, authController.SignIn)
	langGroup.Get("/recover", allowIfNotLoggedInMiddleware, authController.Recover)
	langGroup.Post("/recover", allowIfNotLoggedInMiddleware, authController.Request)
	langGroup.Get("/reset-password", allowIfNotLoggedInMiddleware, authController.EditPassword)
	langGroup.Post("/reset-password", allowIfNotLoggedInMiddleware, authController.UpdatePassword)

	alwaysRequireAuthenticationMiddleware := jwtware.New(jwtware.Config{
		SigningKey:    cfg.JwtSecret,
		SigningMethod: "HS256",
		TokenLookup:   "cookie:coreander",
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Redirect(fmt.Sprintf("/%s/login", chooseBestLanguage(c, supportedLanguages)))
		},
	})

	usersGroup := langGroup.Group("/users", alwaysRequireAuthenticationMiddleware)

	usersController := controller.NewUsers(usersRepository, cfg.MinPasswordLength, cfg.WordsPerMinute)

	usersGroup.Get("/", usersController.List)
	usersGroup.Get("/new", usersController.New)
	usersGroup.Post("/new", usersController.Create)
	usersGroup.Get("/:uuid<guid>/edit", usersController.Edit)
	usersGroup.Post("/:uuid<guid>/edit", usersController.Update)
	usersGroup.Post("/delete", usersController.Delete)

	// Authentication requirement is configurable for all routes below this middleware
	app.Use(jwtware.New(jwtware.Config{
		SigningKey:    cfg.JwtSecret,
		SigningMethod: "HS256",
		TokenLookup:   "cookie:coreander",
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			err = c.Next()
			if cfg.RequireAuth {
				return c.Redirect(fmt.Sprintf("/%s/login", chooseBestLanguage(c, supportedLanguages)))
			}
			return err
		},
	}))

	langGroup.Get("/logout", authController.SignOut)

	app.Get("/covers/:slug", func(c *fiber.Ctx) error {
		return controller.Covers(c, cfg.HomeDir, cfg.LibraryPath, metadataReaders, cfg.CoverMaxWidth, idx)
	})

	app.Post("/send", func(c *fiber.Ctx) error {
		return controller.Send(c, cfg.LibraryPath, sender, idx)
	})

	app.Get("/download/:slug", func(c *fiber.Ctx) error {
		return controller.Download(c, cfg.HomeDir, cfg.LibraryPath, idx)
	})

	langGroup.Get("/", func(c *fiber.Ctx) error {
		session := jwtclaimsreader.SessionData(c)
		wordsPerMinute := session.WordsPerMinute
		if wordsPerMinute == 0 {
			wordsPerMinute = cfg.WordsPerMinute
		}
		return controller.Search(c, idx, cfg.Version, sender, wordsPerMinute)
	})

	langGroup.Get("/read/:slug", func(c *fiber.Ctx) error {
		return controller.DocReader(c, cfg.LibraryPath, idx)
	})

	app.Get("/", func(c *fiber.Ctx) error {
		return controller.Root(c)
	})
}

func getSupportedLanguages(printers map[string]*message.Printer) []string {
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
