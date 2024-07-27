package webserver

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/svera/coreander/v4/internal/webserver/controller"
)

func routes(app *fiber.App, controllers Controllers, jwtSecret []byte, sender Sender, requireAuth bool) {
	var allowIfNotLoggedIn = AllowIfNotLoggedIn(jwtSecret)
	var alwaysRequireAuthentication = AlwaysRequireAuthentication(jwtSecret, sender)
	var configurableAuthentication = ConfigurableAuthentication(jwtSecret, sender, requireAuth)

	app.Use("/css", filesystem.New(filesystem.Config{
		Root: http.FS(cssFS),
	}))

	app.Use("/js", filesystem.New(filesystem.Config{
		Root: http.FS(jsFS),
	}))

	app.Use("/images", filesystem.New(filesystem.Config{
		Root: http.FS(imagesFS),
	}))

	app.Use(func(c *fiber.Ctx) error {
		c.Locals("Version", c.App().Config().AppName)
		c.Locals("SupportedLanguages", supportedLanguages)
		return c.Next()
	})

	langGroup := app.Group(fmt.Sprintf("/:lang<regex(%s)>", strings.Join(supportedLanguages, "|")), func(c *fiber.Ctx) error {
		pathMinusLang := c.Path()[3:]
		query := string(c.Request().URI().QueryString())
		if query != "" {
			pathMinusLang = pathMinusLang + "?" + query
		}
		c.Locals("Lang", c.Params("lang"))
		c.Locals("PathMinusLang", pathMinusLang)
		return c.Next()
	})

	langGroup.Get("/sessions/new", allowIfNotLoggedIn, controllers.Auth.Login)
	langGroup.Post("/sessions", allowIfNotLoggedIn, controllers.Auth.SignIn)
	langGroup.Get("/recover", allowIfNotLoggedIn, controllers.Auth.Recover)
	langGroup.Post("/recover", allowIfNotLoggedIn, controllers.Auth.Request)
	langGroup.Get("/reset-password", allowIfNotLoggedIn, controllers.Auth.EditPassword)
	langGroup.Post("/reset-password", allowIfNotLoggedIn, controllers.Auth.UpdatePassword)
	langGroup.Delete("/sessions", alwaysRequireAuthentication, controllers.Auth.SignOut)

	usersGroup := langGroup.Group("/users", alwaysRequireAuthentication)

	usersGroup.Get("/", alwaysRequireAuthentication, RequireAdmin, controllers.Users.List)
	usersGroup.Get("/new", alwaysRequireAuthentication, RequireAdmin, controllers.Users.New)
	usersGroup.Post("/", alwaysRequireAuthentication, RequireAdmin, controllers.Users.Create)
	usersGroup.Get("/:username", alwaysRequireAuthentication, controllers.Users.Edit)
	usersGroup.Put("/:username", alwaysRequireAuthentication, controllers.Users.Update)
	app.Delete("/users/:username", alwaysRequireAuthentication, RequireAdmin, controllers.Users.Delete)

	langGroup.Get("/stars", alwaysRequireAuthentication, controllers.Highlights.List)
	app.Post("/stars/:slug", alwaysRequireAuthentication, controllers.Highlights.Create)
	app.Delete("/stars/:slug", alwaysRequireAuthentication, controllers.Highlights.Delete)

	app.Delete("/documents/:slug", alwaysRequireAuthentication, RequireAdmin, controllers.Documents.Delete)

	langGroup.Get("/upload", alwaysRequireAuthentication, RequireAdmin, controllers.Documents.UploadForm)
	langGroup.Post("/documents", alwaysRequireAuthentication, RequireAdmin, controllers.Documents.Upload)

	// Authentication requirement is configurable for all routes below this middleware
	langGroup.Use(configurableAuthentication)
	app.Use(configurableAuthentication)

	app.Get("/documents/:slug/cover", controllers.Documents.Cover)
	langGroup.Get("/documents/:slug/read", controllers.Documents.Reader)
	app.Get("/documents/:slug/download", controllers.Documents.Download)

	langGroup.Get("/documents/:slug", controllers.Documents.Detail)

	app.Post("/send", controllers.Documents.Send)

	langGroup.Get("/documents", controllers.Documents.Search)
	langGroup.Get("/", controllers.Documents.Search)

	app.Get("/", func(c *fiber.Ctx) error {
		return controller.Root(c, supportedLanguages)
	})
}
