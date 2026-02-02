package webserver

import (
	"fmt"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/svera/coreander/v4/internal/i18n"
	"github.com/svera/coreander/v4/internal/webserver/model"
	"github.com/svera/coreander/v4/internal/webserver/view"
)

func routes(app *fiber.App, controllers Controllers, jwtSecret []byte, sender Sender, translator i18n.Translator, cfg Config, idx ProgressInfo, usersRepository *model.UserRepository) {
	// Middlewares
	var (
		allowIfNotLoggedIn          = AllowIfNotLoggedIn(jwtSecret)
		alwaysRequireAuthentication = AlwaysRequireAuthentication(jwtSecret, sender, translator, usersRepository)
		configurableAuthentication  = ConfigurableAuthentication(jwtSecret, sender, translator, cfg.RequireAuth, usersRepository)
	)

	staticCacheControl := fmt.Sprintf("public, max-age=%d, immutable", cfg.ClientStaticCacheTTL)
	staticCacheTime := fmt.Sprintf("%d", cfg.ServerStaticCacheTTL)

	app.Use("/css", func(c *fiber.Ctx) error {
		// Set cache control headers for CSS and font files
		c.Set("Cache-Control", staticCacheControl)
		c.Append("Cache-Time", staticCacheTime)
		return c.Next()
	}, filesystem.New(filesystem.Config{
		Root: http.FS(cssFS),
	}))

	app.Use("/js", func(c *fiber.Ctx) error {
		// Set cache control headers for JS files
		c.Set("Cache-Control", staticCacheControl)
		c.Append("Cache-Time", staticCacheTime)
		return c.Next()
	}, filesystem.New(filesystem.Config{
		Root: http.FS(jsFS),
	}))

	app.Use("/images", func(c *fiber.Ctx) error {
		// Set cache control headers for image files
		c.Set("Cache-Control", staticCacheControl)
		c.Append("Cache-Time", staticCacheTime)
		return c.Next()
	}, filesystem.New(filesystem.Config{
		Root: http.FS(imagesFS),
	}))

	app.Use(func(c *fiber.Ctx) error {
		c.Locals("Version", c.App().Config().AppName)
		c.Locals("SupportedLanguages", supportedLanguages)
		c.Locals("Lang", chooseBestLanguage(c))
		q := c.Queries()
		delete(q, "l")
		c.Locals("URLPath", c.Path())
		c.Locals("QueryString", view.ToQueryString(q))
		return c.Next()
	})

	// Set available languages for the language filter
	app.Use(SetAvailableLanguages(idx))

	// Update user's language preference when language changes (runs after authentication)
	app.Use(UpdateUserLanguage(usersRepository))

	app.Get("/sessions/new", allowIfNotLoggedIn, controllers.Auth.Login)
	app.Post("/sessions", allowIfNotLoggedIn, controllers.Auth.SignIn)
	app.Get("/recover", allowIfNotLoggedIn, controllers.Auth.Recover)
	app.Post("/recover", allowIfNotLoggedIn, controllers.Auth.Request)
	app.Get("/reset-password", allowIfNotLoggedIn, controllers.Auth.EditPassword)
	app.Post("/reset-password", allowIfNotLoggedIn, controllers.Auth.UpdatePassword)
	app.Delete("/sessions", alwaysRequireAuthentication, controllers.Auth.SignOut)

	// Public routes for invitation acceptance (must be before usersGroup)
	app.Get("/invite", allowIfNotLoggedIn, controllers.Users.AcceptInviteForm)
	app.Post("/invite", allowIfNotLoggedIn, controllers.Users.AcceptInvite)

	usersGroup := app.Group("/users", alwaysRequireAuthentication)

	usersGroup.Get("/", RequireAdmin, controllers.Users.List)
	usersGroup.Get("/new", RequireAdmin, controllers.Users.New)
	usersGroup.Post("/", RequireAdmin, controllers.Users.Create)
	usersGroup.Get("/invite", RequireAdmin, controllers.Users.InviteForm)
	usersGroup.Post("/invite", RequireAdmin, controllers.Users.SendInvite)
	usersGroup.Get("/share-recipients", controllers.Users.ShareRecipients)
	usersGroup.Get("/:username", controllers.Users.Edit)
	usersGroup.Put("/:username", controllers.Users.Update)
	usersGroup.Delete("/:username", RequireAdmin, controllers.Users.Delete)

	highlightsGroup := app.Group("/highlights", alwaysRequireAuthentication)
	highlightsGroup.Get("/", controllers.Highlights.List)
	highlightsGroup.Post("/:slug", controllers.Highlights.Create)
	highlightsGroup.Delete("/:slug", controllers.Highlights.Delete)

	docsGroup := app.Group("/documents")
	app.Get("/upload", alwaysRequireAuthentication, RequireAdmin, controllers.Documents.UploadForm)
	docsGroup.Post("/", alwaysRequireAuthentication, RequireAdmin, controllers.Documents.Upload)
	docsGroup.Delete("/:slug", alwaysRequireAuthentication, RequireAdmin, controllers.Documents.Delete)

	// Authentication requirement is configurable for all routes below this middleware
	app.Use(configurableAuthentication)

	docsGroup.Get("/:slug/cover", controllers.Documents.Cover)
	docsGroup.Get("/:slug/read", controllers.Documents.Reader)
	docsGroup.Get("/:slug/position", alwaysRequireAuthentication, controllers.Documents.GetPosition)
	docsGroup.Put("/:slug/position", alwaysRequireAuthentication, controllers.Documents.UpdatePosition)
	docsGroup.Post("/:slug/complete", alwaysRequireAuthentication, controllers.Documents.ToggleComplete)
	docsGroup.Put("/:slug/complete", alwaysRequireAuthentication, controllers.Documents.ToggleComplete)
	docsGroup.Get("/:slug/download", controllers.Documents.Download)
	docsGroup.Post("/:slug/send", alwaysRequireAuthentication, controllers.Documents.Send)
	docsGroup.Post("/:slug/share", alwaysRequireAuthentication, controllers.Documents.Share)
	docsGroup.Get("/:slug", controllers.Documents.Detail)
	docsGroup.Get("/", controllers.Documents.Search)

	app.Get("/subjects", controllers.Documents.Subjects)

	app.Get("/authors/:slug.:extension<regex(jpg)$/i>", controllers.Authors.Image)
	app.Get("/authors/:slug", controllers.Authors.Documents)
	app.Get("/authors/:slug/summary", controllers.Authors.Summary)
	app.Put("/authors/:slug", controllers.Authors.Update, alwaysRequireAuthentication, RequireAdmin)
	app.Post("/authors/:slug/image", alwaysRequireAuthentication, RequireAdmin, controllers.Authors.UploadImage)

	app.Get("/series/:slug", controllers.Series.Documents)

	app.Get("/", controllers.Home.Index)
}
