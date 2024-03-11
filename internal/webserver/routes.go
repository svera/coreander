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
	var allowIfNotLoggedInMiddleware = AllowIfNotLoggedIn(jwtSecret)
	var alwaysRequireAuthenticationMiddleware = AlwaysRequireAuthentication(jwtSecret, sender)
	var configurableAuthenticationMiddleware = ConfigurableAuthentication(jwtSecret, sender, requireAuth)

	app.Use("/css", filesystem.New(filesystem.Config{
		Root: http.FS(cssFS),
	}))

	app.Use("/js", filesystem.New(filesystem.Config{
		Root: http.FS(jsFS),
	}))

	app.Use("/images", filesystem.New(filesystem.Config{
		Root: http.FS(imagesFS),
	}))

	langGroup := app.Group(fmt.Sprintf("/:lang<regex(%s)>", strings.Join(getSupportedLanguages(), "|")), func(c *fiber.Ctx) error {
		pathMinusLang := c.Path()[3:]
		query := string(c.Request().URI().QueryString())
		if query != "" {
			pathMinusLang = pathMinusLang + "?" + query
		}
		c.Locals("Lang", c.Params("lang"))
		c.Locals("SupportedLanguages", getSupportedLanguages())
		c.Locals("PathMinusLang", pathMinusLang)
		c.Locals("Version", c.App().Config().AppName)
		return c.Next()
	})

	langGroup.Get("/login", allowIfNotLoggedInMiddleware, controllers.Auth.Login)
	langGroup.Post("login", allowIfNotLoggedInMiddleware, controllers.Auth.SignIn)
	langGroup.Get("/recover", allowIfNotLoggedInMiddleware, controllers.Auth.Recover)
	langGroup.Post("/recover", allowIfNotLoggedInMiddleware, controllers.Auth.Request)
	langGroup.Get("/reset-password", allowIfNotLoggedInMiddleware, controllers.Auth.EditPassword)
	langGroup.Post("/reset-password", allowIfNotLoggedInMiddleware, controllers.Auth.UpdatePassword)

	usersGroup := langGroup.Group("/users", alwaysRequireAuthenticationMiddleware)

	usersGroup.Get("/", RequireAdmin, controllers.Users.List)
	usersGroup.Get("/new", RequireAdmin, controllers.Users.New)
	usersGroup.Post("/new", RequireAdmin, controllers.Users.Create)
	usersGroup.Get("/:uuid<guid>/edit", controllers.Users.Edit)
	usersGroup.Post("/:uuid<guid>/edit", controllers.Users.Update)
	usersGroup.Post("/delete", RequireAdmin, controllers.Users.Delete)

	langGroup.Get("/highlights/:uuid<guid>", alwaysRequireAuthenticationMiddleware, controllers.Highlights.Highlights)
	app.Post("/highlights", alwaysRequireAuthenticationMiddleware, controllers.Highlights.Highlight)
	app.Delete("/highlights", alwaysRequireAuthenticationMiddleware, controllers.Highlights.Remove)

	app.Post("/delete", alwaysRequireAuthenticationMiddleware, RequireAdmin, controllers.Documents.Delete)

	langGroup.Get("/upload", alwaysRequireAuthenticationMiddleware, RequireAdmin, controllers.Documents.UploadForm)
	langGroup.Post("/upload", alwaysRequireAuthenticationMiddleware, RequireAdmin, controllers.Documents.Upload)

	// Authentication requirement is configurable for all routes below this middleware
	app.Use(configurableAuthenticationMiddleware)

	langGroup.Get("/logout", controllers.Auth.SignOut)

	app.Get("/cover/:slug", controllers.Documents.Cover)

	langGroup.Get("/document/:slug", controllers.Documents.Detail)

	app.Post("/send", controllers.Documents.Send)

	app.Get("/download/:slug", controllers.Documents.Download)

	langGroup.Get("/", controllers.Documents.Search)

	langGroup.Get("/read/:slug", controllers.Documents.Reader)

	app.Get("/", func(c *fiber.Ctx) error {
		return controller.Root(c, getSupportedLanguages())
	})
}
