package webserver

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cache"
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

type sendAttachentFormData struct {
	File  string `form:"file"`
	Email string `form:"email"`
}

// New builds a new Fiber application and set up the required routes
func New(idx controller.Reader, libraryPath, homeDir, version string, metadataReaders map[string]metadata.Reader, coverMaxWidth int, sender controller.Sender, db *gorm.DB) *fiber.App {
	engine, err := initTemplateEngine()
	if err != nil {
		log.Fatal(err)
	}

	app := fiber.New(fiber.Config{
		Views:                 engine,
		DisableStartupMessage: true,
	})

	app.Use(cache.New(cache.Config{
		ExpirationGenerator: func(c *fiber.Ctx, cfg *cache.Config) time.Duration {
			newCacheTime, _ := strconv.Atoi(c.GetRespHeader("Cache-Time", "0"))
			return time.Second * time.Duration(newCacheTime)
		},
		//CacheControl: true,
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

	app.Get("/covers/:filename", func(c *fiber.Ctx) error {
		c.Append("Cache-Time", "86400")
		return controller.Covers(c, homeDir, libraryPath, metadataReaders, coverMaxWidth, embedded)
	})

	app.Post("/send", func(c *fiber.Ctx) error {
		data := new(sendAttachentFormData)

		if err := c.BodyParser(data); err != nil {
			return err
		}

		controller.Send(c, libraryPath, data.File, data.Email, sender)
		return nil
	})

	app.Get("/:lang/read/:filename", func(c *fiber.Ctx) error {
		return controller.DocReader(c, libraryPath)
	})

	app.Use("/", jwtclaimsreader.New(jwtclaimsreader.Config{}))

	authRepository := &model.Auth{DB: db}
	authController := controller.NewAuth(authRepository, version)

	usersRepository := &model.Users{DB: db}
	usersController := controller.NewUsers(usersRepository, version)

	app.Get("/:lang/login", authController.Login)
	app.Post("/:lang/login", authController.SignIn)
	app.Get("/:lang/logout", authController.SignOut)

	app.Get("/:lang", func(c *fiber.Ctx) error {
		emailSendingConfigured := true
		if _, ok := sender.(*infrastructure.NoEmail); ok {
			emailSendingConfigured = false
		}
		return controller.Search(c, idx, version, emailSendingConfigured)
	})

	app.Get("/", func(c *fiber.Ctx) error {
		return controller.Root(c)
	})

	app.Static("/files", libraryPath)

	// JWT Middleware
	app.Use("/:lang/", jwtware.New(jwtware.Config{
		SigningKey:    []byte(os.Getenv("JWT_SECRET")),
		SigningMethod: "HS256",
		TokenLookup:   "cookie:jwt",
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Redirect(fmt.Sprintf("/%s/login", c.Params("lang", "en")))
		},
		/*
			ErrorHandler: func(c *fiber.Ctx, err error) error {
				if err.Error() == "Missing or malformed JWT" {
					return c.Status(fiber.StatusBadRequest).SendString("Missing or malformed JWT")
				}
				return c.Status(fiber.StatusUnauthorized).SendString("Invalid or expired JWT")
			},
		*/
	}))

	app.Get("/:lang/users/edit/:uuid", usersController.Edit)
	app.Get("/:lang/users", usersController.List)
	app.Get("/:lang/users/new", usersController.New)
	app.Post("/:lang/users/create", usersController.Create)

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
