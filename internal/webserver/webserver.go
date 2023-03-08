package webserver

import (
	"embed"
	"errors"
	"fmt"
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
	"github.com/svera/coreander/internal/controller"
	"github.com/svera/coreander/internal/i18n"
	"github.com/svera/coreander/internal/infrastructure"
	"github.com/svera/coreander/internal/jwtclaimsreader"
	"github.com/svera/coreander/internal/metadata"
	"github.com/svera/coreander/internal/model"
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
	Hostname          string
	Port              string
}

type Sender interface {
	Send(address, subject, body string) error
	SendDocument(address string, libraryPath string, fileName string) error
}

// New builds a new Fiber application and set up the required routes
func New(idx controller.Reader, cfg Config, metadataReaders map[string]metadata.Reader, sender Sender, db *gorm.DB) *fiber.App {
	viewsFS, err := fs.Sub(embedded, "embedded/views")
	if err != nil {
		log.Fatal(err)
	}

	printers, err := i18n.Printers(embedded)
	if err != nil {
		log.Fatal(err)
	}
	engine, err := infrastructure.TemplateEngine(viewsFS, printers)
	if err != nil {
		log.Fatal(err)
	}

	emailSendingConfigured := true
	if _, ok := sender.(*infrastructure.NoEmail); ok {
		emailSendingConfigured = false
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

	usersRepository := &model.UserRepository{DB: db}

	authCfg := controller.AuthConfig{
		MinPasswordLength: cfg.MinPasswordLength,
		Secret:            cfg.JwtSecret,
		Hostname:          cfg.Hostname,
		Port:              cfg.Port,
	}

	authController := controller.NewAuth(usersRepository, sender, authCfg, printers)

	langGroup := app.Group("/:lang<regex(es|en)>")

	// JWT Middleware
	app.Use(jwtware.New(jwtware.Config{
		SigningKey:    cfg.JwtSecret,
		SigningMethod: "HS256",
		TokenLookup:   "cookie:coreander",
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			err = c.Next()
			if cfg.RequireAuth &&
				!strings.HasPrefix(c.Route().Path, "/:lang<regex(es|en)>/login") &&
				!strings.HasPrefix(c.Route().Path, "/:lang<regex(es|en)>/recover") &&
				!strings.HasPrefix(c.Route().Path, "/:lang<regex(es|en)>/reset-password") {
				return c.Redirect(fmt.Sprintf("/%s/login", c.Params("lang", "en")))
			}
			if strings.HasPrefix(c.Route().Path, "/:lang<regex(es|en)>/users") {
				return c.Redirect(fmt.Sprintf("/%s/login", c.Params("lang", "en")))
			}
			return err
		},
	}))

	langGroup.Get("/login", authController.Login)
	langGroup.Post("login", authController.SignIn)
	langGroup.Get("/logout", authController.SignOut)
	langGroup.Get("/recover", authController.Recover)
	langGroup.Post("/recover", authController.Request)
	langGroup.Get("/reset-password", authController.EditPassword)
	langGroup.Post("/reset-password", authController.UpdatePassword)

	app.Get("/covers/:filename", func(c *fiber.Ctx) error {
		c.Append("Cache-Time", "86400")
		return controller.Covers(c, cfg.HomeDir, cfg.LibraryPath, metadataReaders, cfg.CoverMaxWidth, embedded)
	})

	app.Post("/send", func(c *fiber.Ctx) error {
		if c.FormValue("file") == "" || c.FormValue("email") == "" {
			return fiber.ErrBadRequest
		}
		controller.Send(c, cfg.LibraryPath, c.FormValue("file"), c.FormValue("email"), sender)
		return nil
	})

	app.Static("/files", cfg.LibraryPath)

	usersController := controller.NewUsers(usersRepository, cfg.MinPasswordLength, cfg.WordsPerMinute)

	langGroup.Get("/", func(c *fiber.Ctx) error {
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

	langGroup.Get("/users", usersController.List)
	langGroup.Get("/users/new", usersController.New)
	langGroup.Post("/users/new", usersController.Create)
	langGroup.Get("/users/:uuid<guid>/edit", usersController.Edit)
	langGroup.Post("/users/:uuid<guid>/edit", usersController.Update)
	langGroup.Post("/users/delete", usersController.Delete)

	app.Get("/", func(c *fiber.Ctx) error {
		return controller.Root(c)
	})

	return app
}
