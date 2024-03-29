package webserver

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/svera/coreander/v3/internal/webserver/controller"
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

	langGroup.Get("/login", allowIfNotLoggedIn, controllers.Auth.Login)
	langGroup.Post("login", allowIfNotLoggedIn, controllers.Auth.SignIn)
	langGroup.Get("/recover", allowIfNotLoggedIn, controllers.Auth.Recover)
	langGroup.Post("/recover", allowIfNotLoggedIn, controllers.Auth.Request)
	langGroup.Get("/reset-password", allowIfNotLoggedIn, controllers.Auth.EditPassword)
	langGroup.Post("/reset-password", allowIfNotLoggedIn, controllers.Auth.UpdatePassword)

	usersGroup := langGroup.Group("/users", alwaysRequireAuthentication)

	usersGroup.Get("/", alwaysRequireAuthentication, RequireAdmin, controllers.Users.List)
	usersGroup.Get("/new", alwaysRequireAuthentication, RequireAdmin, controllers.Users.New)
	usersGroup.Post("/new", alwaysRequireAuthentication, RequireAdmin, controllers.Users.Create)
	usersGroup.Get("/:uuid<guid>/edit", alwaysRequireAuthentication, controllers.Users.Edit)
	usersGroup.Post("/:uuid<guid>/edit", alwaysRequireAuthentication, controllers.Users.Update)
	app.Delete("/users", alwaysRequireAuthentication, RequireAdmin, controllers.Users.Delete)

	langGroup.Get("/highlights/:uuid<guid>", alwaysRequireAuthentication, controllers.Highlights.Highlights)
	app.Post("/highlights", alwaysRequireAuthentication, controllers.Highlights.Highlight)
	app.Delete("/highlights", alwaysRequireAuthentication, controllers.Highlights.Remove)

	app.Delete("/document", alwaysRequireAuthentication, RequireAdmin, controllers.Documents.Delete)

	langGroup.Get("/upload", alwaysRequireAuthentication, RequireAdmin, controllers.Documents.UploadForm)
	langGroup.Post("/upload", alwaysRequireAuthentication, RequireAdmin, controllers.Documents.Upload)

	// Authentication requirement is configurable for all routes below this middleware
	app.Use(configurableAuthentication)

	langGroup.Get("/logout", controllers.Auth.SignOut)

	app.Get("/cover/:slug", controllers.Documents.Cover)

	langGroup.Get("/document/:slug", controllers.Documents.Detail)

	app.Post("/send", controllers.Documents.Send)

	app.Get("/download/:slug", controllers.Documents.Download)

	langGroup.Get("/", controllers.Documents.Search)

	langGroup.Get("/read/:slug", controllers.Documents.Reader)

	app.Get("/", func(c *fiber.Ctx) error {
		return controller.Root(c, supportedLanguages)
	})
}
