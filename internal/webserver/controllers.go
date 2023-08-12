package webserver

import (
	"errors"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	jwtware "github.com/gofiber/jwt/v3"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v3/internal/controller"
	"github.com/svera/coreander/v3/internal/index"
	"github.com/svera/coreander/v3/internal/jwtclaimsreader"
	"github.com/svera/coreander/v3/internal/metadata"
	"github.com/svera/coreander/v3/internal/model"
	"gorm.io/gorm"
)

type Controllers struct {
	Auth                                  *controller.Auth
	Users                                 *controller.Users
	Cover                                 func(c *fiber.Ctx) error
	Send                                  func(c *fiber.Ctx) error
	Download                              func(c *fiber.Ctx) error
	Read                                  func(c *fiber.Ctx) error
	Detail                                func(c *fiber.Ctx) error
	Delete                                func(c *fiber.Ctx) error
	Search                                func(c *fiber.Ctx) error
	AllowIfNotLoggedInMiddleware          func(c *fiber.Ctx) error
	AlwaysRequireAuthenticationMiddleware func(c *fiber.Ctx) error
	ConfigurableAuthenticationMiddleware  func(c *fiber.Ctx) error
	ErrorHandler                          func(c *fiber.Ctx, err error) error
}

func SetupControllers(cfg Config, db *gorm.DB, metadataReaders map[string]metadata.Reader, idx *index.BleveIndexer, sender Sender, appFs afero.Fs) Controllers {
	usersRepository := &model.UserRepository{DB: db}

	authCfg := controller.AuthConfig{
		MinPasswordLength: cfg.MinPasswordLength,
		Secret:            cfg.JwtSecret,
		Hostname:          cfg.Hostname,
		Port:              cfg.Port,
		SessionTimeout:    cfg.SessionTimeout,
	}

	usersCfg := controller.UsersConfig{
		MinPasswordLength: cfg.MinPasswordLength,
		WordsPerMinute:    cfg.WordsPerMinute,
	}

	authController := controller.NewAuth(usersRepository, sender, authCfg, printers)
	usersController := controller.NewUsers(usersRepository, usersCfg)

	return Controllers{
		Auth:  authController,
		Users: usersController,
		Cover: func(c *fiber.Ctx) error {
			return controller.Cover(c, cfg.HomeDir, cfg.LibraryPath, metadataReaders, cfg.CoverMaxWidth, idx)
		},
		Send: func(c *fiber.Ctx) error {
			return controller.Send(c, cfg.LibraryPath, sender, idx)
		},
		Download: func(c *fiber.Ctx) error {
			return controller.Download(c, cfg.HomeDir, cfg.LibraryPath, idx)
		},
		Read: func(c *fiber.Ctx) error {
			return controller.DocReader(c, cfg.LibraryPath, idx)
		},
		Detail: func(c *fiber.Ctx) error {
			return controller.Detail(c, cfg.LibraryPath, sender, idx, cfg.WordsPerMinute)
		},
		Delete: func(c *fiber.Ctx) error {
			return controller.Delete(c, cfg.LibraryPath, idx, appFs)
		},
		Search: func(c *fiber.Ctx) error {
			return controller.Search(c, idx, sender, cfg.WordsPerMinute)
		},
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
				return c.Redirect(fmt.Sprintf("/%s/login", chooseBestLanguage(c, getSupportedLanguages())))
			},
		}),
		ConfigurableAuthenticationMiddleware: jwtware.New(jwtware.Config{
			SigningKey:    cfg.JwtSecret,
			SigningMethod: "HS256",
			TokenLookup:   "cookie:coreander",
			ErrorHandler: func(c *fiber.Ctx, err error) error {
				err = c.Next()
				if cfg.RequireAuth {
					return c.Redirect(fmt.Sprintf("/%s/login", chooseBestLanguage(c, getSupportedLanguages())))
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
					"Lang":    chooseBestLanguage(c, getSupportedLanguages()),
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
