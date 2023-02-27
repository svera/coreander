package webserver

import (
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cache"
	"github.com/gofiber/fiber/v2/middleware/favicon"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	jwtware "github.com/gofiber/jwt/v3"
	fibertpl "github.com/gofiber/template/html"
	"github.com/svera/coreander/internal/controller"
	"github.com/svera/coreander/internal/i18n"
	"github.com/svera/coreander/internal/infrastructure"
	"github.com/svera/coreander/internal/jwtclaimsreader"
	"github.com/svera/coreander/internal/metadata"
	"github.com/svera/coreander/internal/model"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"gorm.io/gorm"
)

//go:embed embedded
var embedded embed.FS

type Config struct {
	LibraryPath       string
	HomeDir           string
	Version           string
	CoverMaxWidth     int
	JwtSecret         []byte
	RequireAuth       bool
	MinPasswordLength int
	WordsPerMinute    float64
}

// New builds a new Fiber application and set up the required routes
func New(idx controller.Reader, cfg Config, metadataReaders map[string]metadata.Reader, sender controller.Sender, db *gorm.DB) *fiber.App {
	engine, err := initTemplateEngine()
	if err != nil {
		log.Fatal(err)
	}

	app := fiber.New(fiber.Config{
		Views:                 engine,
		DisableStartupMessage: true,
		AppName:               cfg.Version,
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			// Status code defaults to 500
			code := fiber.StatusInternalServerError

			// Retrieve the custom status code if it's a *fiber.Error
			var e *fiber.Error
			if errors.As(err, &e) {
				code = e.Code
			}

			// Send custom error page
			err = ctx.Status(code).Render(
				fmt.Sprintf("errors/%d", code),
				fiber.Map{
					"Lang":    ctx.Params("lang", "en"),
					"Title":   "Coreander",
					"Session": jwtclaimsreader.SessionData(ctx),
					"Version": ctx.App().Config().AppName,
				},
				"layout")

			if err != nil {
				log.Println(err)
				// In case the Render fails
				return ctx.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
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

	// JWT Middleware
	app.Use(jwtware.New(jwtware.Config{
		SigningKey:    cfg.JwtSecret,
		SigningMethod: "HS256",
		TokenLookup:   "cookie:coreander",
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			err = c.Next()
			if cfg.RequireAuth && !strings.HasPrefix(c.Route().Path, "/:lang<regex(es|en)>/login") {
				return c.Redirect(fmt.Sprintf("/%s/login", c.Params("lang", "en")))
			}
			if strings.HasPrefix(c.Route().Path, "/:lang<regex(es|en)>/users") {
				return c.Redirect(fmt.Sprintf("/%s/login", c.Params("lang", "en")))
			}
			return err
		},
	}))

	app.Get("/covers/:filename", func(c *fiber.Ctx) error {
		c.Append("Cache-Time", "86400")
		return controller.Covers(c, cfg.HomeDir, cfg.LibraryPath, metadataReaders, cfg.CoverMaxWidth, embedded)
	})

	app.Post("/send", func(c *fiber.Ctx) error {
		controller.Send(c, cfg.LibraryPath, c.FormValue("file"), c.FormValue("email"), sender)
		return nil
	})

	app.Static("/files", cfg.LibraryPath)

	usersRepository := &model.UserRepository{DB: db}

	authController := controller.NewAuth(usersRepository, cfg.JwtSecret)
	usersController := controller.NewUsers(usersRepository, cfg.MinPasswordLength, cfg.WordsPerMinute)

	langGroup := app.Group("/:lang<regex(es|en)>")

	langGroup.Get("/", func(c *fiber.Ctx) error {
		emailSendingConfigured := true
		if _, ok := sender.(*infrastructure.NoEmail); ok {
			emailSendingConfigured = false
		}
		session := jwtclaimsreader.SessionData(c)
		wordsPerMinute := session.WordsPerMinute
		if wordsPerMinute == 0 {
			wordsPerMinute = cfg.WordsPerMinute
		}
		return controller.Search(c, idx, cfg.Version, emailSendingConfigured, wordsPerMinute)
	})

	langGroup.Get("/read/:filename", func(c *fiber.Ctx) error {
		return controller.DocReader(c, cfg.LibraryPath)
	})

	langGroup.Get("/login", authController.Login)
	langGroup.Post("login", authController.SignIn)
	langGroup.Get("/logout", authController.SignOut)

	langGroup.Get("/users/:uuid/edit", usersController.Edit)
	langGroup.Post("/users/:uuid/edit", usersController.Update)
	langGroup.Post("/users/:uuid/update-password", usersController.UpdatePassword)
	langGroup.Get("/users", usersController.List)
	langGroup.Get("/users/new", usersController.New)
	langGroup.Post("/users/new", usersController.Create)
	langGroup.Post("/users/delete", usersController.Delete)

	app.Get("/", func(c *fiber.Ctx) error {
		return controller.Root(c)
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
		"en": message.NewPrinter(language.English),
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
