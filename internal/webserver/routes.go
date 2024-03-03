package webserver

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/svera/coreander/v3/internal/webserver/controller"
)

func routes(app *fiber.App, controllers Controllers, supportedLanguages []string) {
	app.Use("/css", filesystem.New(filesystem.Config{
		Root: http.FS(cssFS),
	}))

	app.Use("/js", filesystem.New(filesystem.Config{
		Root: http.FS(jsFS),
	}))

	app.Use("/images", filesystem.New(filesystem.Config{
		Root: http.FS(imagesFS),
	}))

	langGroup := app.Group(fmt.Sprintf("/:lang<regex(%s)>", strings.Join(supportedLanguages, "|")), func(c *fiber.Ctx) error {
		pathMinusLang := c.Path()[3:]
		query := string(c.Request().URI().QueryString())
		if query != "" {
			pathMinusLang = pathMinusLang + "?" + query
		}
		c.Locals("Lang", c.Params("lang"))
		c.Locals("SupportedLanguages", supportedLanguages)
		c.Locals("PathMinusLang", pathMinusLang)
		c.Locals("Version", c.App().Config().AppName)
		return c.Next()
	})

	langGroup.Get("/login", controllers.AllowIfNotLoggedInMiddleware, controllers.Auth.Login)
	langGroup.Post("login", controllers.AllowIfNotLoggedInMiddleware, controllers.Auth.SignIn)
	langGroup.Get("/recover", controllers.AllowIfNotLoggedInMiddleware, controllers.Auth.Recover)
	langGroup.Post("/recover", controllers.AllowIfNotLoggedInMiddleware, controllers.Auth.Request)
	langGroup.Get("/reset-password", controllers.AllowIfNotLoggedInMiddleware, controllers.Auth.EditPassword)
	langGroup.Post("/reset-password", controllers.AllowIfNotLoggedInMiddleware, controllers.Auth.UpdatePassword)

	usersGroup := langGroup.Group("/users", controllers.AlwaysRequireAuthenticationMiddleware)

	usersGroup.Get("/", controllers.Users.List)
	usersGroup.Get("/new", controllers.Users.New)
	usersGroup.Post("/new", controllers.Users.Create)
	usersGroup.Get("/:uuid<guid>/edit", controllers.Users.Edit)
	usersGroup.Post("/:uuid<guid>/edit", controllers.Users.Update)
	usersGroup.Post("/delete", controllers.Users.Delete)

	langGroup.Get("/highlights/:uuid<guid>", controllers.AlwaysRequireAuthenticationMiddleware, controllers.Highlights.Highlights)
	app.Post("/highlights", controllers.AlwaysRequireAuthenticationMiddleware, controllers.Highlights.Highlight)
	app.Delete("/highlights", controllers.AlwaysRequireAuthenticationMiddleware, controllers.Highlights.Remove)

	app.Post("/delete", controllers.AlwaysRequireAuthenticationMiddleware, controllers.Documents.Delete)

	langGroup.Get("/upload", controllers.AlwaysRequireAuthenticationMiddleware, controllers.Documents.UploadForm)
	langGroup.Post("/upload", controllers.AlwaysRequireAuthenticationMiddleware, controllers.Documents.Upload)

	// Authentication requirement is configurable for all routes below this middleware
	app.Use(controllers.ConfigurableAuthenticationMiddleware)

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
