package webserver

import (
	"errors"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	jwtware "github.com/gofiber/jwt/v3"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/controller/auth"
	"github.com/svera/coreander/v4/internal/controller/document"
	"github.com/svera/coreander/v4/internal/controller/highlight"
	"github.com/svera/coreander/v4/internal/controller/user"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/infrastructure"
	"github.com/svera/coreander/v4/internal/jwtclaimsreader"
	"github.com/svera/coreander/v4/internal/metadata"
	"github.com/svera/coreander/v4/internal/model"
	"gorm.io/gorm"
)

type Controllers struct {
	Auth                                  *auth.Controller
	Users                                 *user.Controller
	Highlights                            *highlight.Controller
	Documents                             *document.Controller
	AllowIfNotLoggedInMiddleware          func(c *fiber.Ctx) error
	AlwaysRequireAuthenticationMiddleware func(c *fiber.Ctx) error
	ConfigurableAuthenticationMiddleware  func(c *fiber.Ctx) error
	ErrorHandler                          func(c *fiber.Ctx, err error) error
}

func SetupControllers(cfg Config, db *gorm.DB, metadataReaders map[string]metadata.Reader, idx *index.BleveIndexer, sender Sender, appFs afero.Fs) Controllers {
	usersRepository := &model.UserRepository{DB: db}
	highlightsRepository := &model.HighlightRepository{DB: db}

	authCfg := auth.Config{
		MinPasswordLength: cfg.MinPasswordLength,
		Secret:            cfg.JwtSecret,
		Hostname:          cfg.Hostname,
		Port:              cfg.Port,
		SessionTimeout:    cfg.SessionTimeout,
	}

	usersCfg := user.Config{
		MinPasswordLength: cfg.MinPasswordLength,
		WordsPerMinute:    cfg.WordsPerMinute,
	}

	documentsCfg := document.Config{
		WordsPerMinute: cfg.WordsPerMinute,
		LibraryPath:    cfg.LibraryPath,
		HomeDir:        cfg.HomeDir,
		CoverMaxWidth:  cfg.CoverMaxWidth,
	}

	authController := auth.NewController(usersRepository, sender, authCfg, printers)
	usersController := user.NewController(usersRepository, usersCfg)
	highlightsController := highlight.NewController(highlightsRepository, usersRepository, sender, cfg.WordsPerMinute, idx)
	documentsController := document.NewController(highlightsRepository, sender, idx, metadataReaders, appFs, documentsCfg)

	emailSendingConfigured := true
	if _, ok := sender.(*infrastructure.NoEmail); ok {
		emailSendingConfigured = false
	}

	supportedLanguages := getSupportedLanguages()

	return Controllers{
		Auth:       authController,
		Users:      usersController,
		Highlights: highlightsController,
		Documents:  documentsController,
		AllowIfNotLoggedInMiddleware: jwtware.New(jwtware.Config{
			SigningKey:    cfg.JwtSecret,
			SigningMethod: "HS256",
			TokenLookup:   "cookie:coreander",
			SuccessHandler: func(c *fiber.Ctx) error {
				return fiber.ErrForbidden
			},
			ErrorHandler: func(c *fiber.Ctx, err error) error {
				return c.Next()
			},
		}),
		AlwaysRequireAuthenticationMiddleware: jwtware.New(jwtware.Config{
			SigningKey:    cfg.JwtSecret,
			SigningMethod: "HS256",
			TokenLookup:   "cookie:coreander",
			ErrorHandler: func(c *fiber.Ctx, err error) error {
				return c.Status(fiber.StatusForbidden).Render("auth/login", fiber.Map{
					"Lang":                   chooseBestLanguage(c, supportedLanguages),
					"Title":                  "Login",
					"Version":                c.App().Config().AppName,
					"EmailSendingConfigured": emailSendingConfigured,
					"SupportedLanguages":     supportedLanguages,
				}, "layout")
			},
		}),
		ConfigurableAuthenticationMiddleware: jwtware.New(jwtware.Config{
			SigningKey:    cfg.JwtSecret,
			SigningMethod: "HS256",
			TokenLookup:   "cookie:coreander",
			ErrorHandler: func(c *fiber.Ctx, err error) error {
				err = c.Next()
				if cfg.RequireAuth {
					return c.Status(fiber.StatusForbidden).Render("auth/login", fiber.Map{
						"Lang":                   chooseBestLanguage(c, supportedLanguages),
						"Title":                  "Login",
						"Version":                c.App().Config().AppName,
						"EmailSendingConfigured": emailSendingConfigured,
						"SupportedLanguages":     supportedLanguages,
					}, "layout")
				}
				return err
			},
		}),
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
	}
}
