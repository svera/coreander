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
	// Middlewares
	var (
		allowIfNotLoggedIn          = AllowIfNotLoggedIn(jwtSecret)
		alwaysRequireAuthentication = AlwaysRequireAuthentication(jwtSecret, sender)
		configurableAuthentication  = ConfigurableAuthentication(jwtSecret, sender, requireAuth)
	)

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

	usersGroup.Get("/", RequireAdmin, controllers.Users.List)
	usersGroup.Get("/new", RequireAdmin, controllers.Users.New)
	usersGroup.Post("/", RequireAdmin, controllers.Users.Create)
	usersGroup.Get("/:username", controllers.Users.Edit)
	usersGroup.Put("/:username", controllers.Users.Update)
	usersGroup.Delete("/:username", RequireAdmin, controllers.Users.Delete)

	highlightsGroup := langGroup.Group("/highlights", alwaysRequireAuthentication)
	highlightsGroup.Get("/", controllers.Highlights.List)
	highlightsGroup.Post("/:slug", controllers.Highlights.Create)
	highlightsGroup.Delete("/:slug", controllers.Highlights.Delete)

	docsGroup := langGroup.Group("/documents")
	langGroup.Get("/upload", alwaysRequireAuthentication, RequireAdmin, controllers.Documents.UploadForm)
	docsGroup.Post("/", alwaysRequireAuthentication, RequireAdmin, controllers.Documents.Upload)
	docsGroup.Delete("/:slug", alwaysRequireAuthentication, RequireAdmin, controllers.Documents.Delete)

	// Authentication requirement is configurable for all routes below this middleware
	langGroup.Use(configurableAuthentication)
	app.Use(configurableAuthentication)

	docsGroup.Get("/:slug/cover", controllers.Documents.Cover)
	docsGroup.Get("/:slug/read", controllers.Documents.Reader)
	docsGroup.Get("/:slug/download", controllers.Documents.Download)
	docsGroup.Post("/:slug/send", controllers.Documents.Send)
	docsGroup.Get("/:slug", controllers.Documents.Detail)
	docsGroup.Get("/", controllers.Documents.Search)

	langGroup.Get("/", controllers.Documents.Search)

	app.Get("/", func(c *fiber.Ctx) error {
		return controller.Root(c, supportedLanguages)
	})
}
