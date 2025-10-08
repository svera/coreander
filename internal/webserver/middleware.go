package webserver

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	jwtware "github.com/gofiber/jwt/v3"
	"github.com/svera/coreander/v4/internal/i18n"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

// RequireAdmin returns HTTP forbidden if the user requesting access
// is not an admin
func RequireAdmin(c *fiber.Ctx) error {
	if c.Locals("Session") == nil {
		return fiber.ErrForbidden
	}

	session := c.Locals("Session").(model.Session)

	if session.Role != model.RoleAdmin {
		return fiber.ErrForbidden
	}

	return c.Next()
}

// SetFQDN composes the Fully Qualified Domain Name of the host running the app and sets it
// as a local variable of the request
func SetFQDN(cfg Config) func(*fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		c.Locals("fqdn", fmt.Sprintf("%s://%s",
			c.Protocol(),
			cfg.FQDN,
		))
		return c.Next()
	}
}

// SetProgress retrieves indexing progress information from the index and sets it
// as a local variable of the request
func SetProgress(progress ProgressInfo) func(*fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		progress, err := progress.IndexingProgress()
		if err != nil {
			fmt.Println(err)
		}
		if progress.RemainingTime > 0 {
			c.Locals("RemainingIndexingTime", fmt.Sprintf("%d", progress.RemainingTime.Round(time.Minute)/time.Minute))
			c.Locals("IndexingProgressPercentage", progress.Percentage)
		}
		return c.Next()
	}
}

// AllowIfNotLoggedIn only allows processing the request if there is no session
func AllowIfNotLoggedIn(jwtSecret []byte) func(*fiber.Ctx) error {
	return jwtware.New(jwtware.Config{
		SigningKey:    jwtSecret,
		SigningMethod: "HS256",
		TokenLookup:   "cookie:session",
		SuccessHandler: func(c *fiber.Ctx) error {
			return fiber.ErrForbidden
		},
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Next()
		},
	})
}

// AlwaysRequireAuthentication returns forbidden and renders the login page
// if the user trying to access has not logged in
func AlwaysRequireAuthentication(jwtSecret []byte, sender Sender, translator i18n.Translator) func(*fiber.Ctx) error {
	return jwtware.New(jwtware.Config{
		SigningKey:    jwtSecret,
		SigningMethod: "HS256",
		TokenLookup:   "cookie:session",
		SuccessHandler: func(c *fiber.Ctx) error {
			c.Locals("Session", sessionData(c))
			return c.Next()
		},
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return forbidden(c, sender, translator, err)
		},
	})
}

// ConfigurableAuthentication allows to enable or disable authentication on routes which may or may not require it
func ConfigurableAuthentication(jwtSecret []byte, sender Sender, translator i18n.Translator, requireAuth bool) func(*fiber.Ctx) error {
	return jwtware.New(jwtware.Config{
		SigningKey:    jwtSecret,
		SigningMethod: "HS256",
		TokenLookup:   "cookie:session",
		SuccessHandler: func(c *fiber.Ctx) error {
			c.Locals("Session", sessionData(c))
			return c.Next()
		},
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			if requireAuth {
				return forbidden(c, sender, translator, err)
			}
			return c.Next()
		},
	})
}

func forbidden(c *fiber.Ctx, sender Sender, translator i18n.Translator, err error) error {
	emailSendingConfigured := true
	if _, ok := sender.(*infrastructure.NoEmail); ok {
		emailSendingConfigured = false
	}
	message := ""
	if err.Error() != "missing or malformed JWT" && c.Cookies("session") != "" {
		message = "Session expired, please log in again."
	}
	return c.Status(fiber.StatusForbidden).Render("auth/login", fiber.Map{
		"Lang":                   chooseBestLanguage(c),
		"Title":                  translator.T(c.Locals("Lang").(string), "Log in"),
		"EmailSendingConfigured": emailSendingConfigured,
		"DisableLoginLink":       true,
		"Warning":                message,
	}, "layout")
}

func OneTimeMessages() func(c *fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		msg := ""
		if c.Cookies("warning-once") != "" {
			msg = c.Cookies("warning-once")
			c.Cookie(&fiber.Cookie{
				Name:    "warning-once",
				Expires: time.Now().Add(-(time.Hour * 2)),
			})
		}
		c.Locals("Warning", msg)

		msg = ""
		if c.Cookies("success-once") != "" {
			msg = c.Cookies("success-once")
			c.Cookie(&fiber.Cookie{
				Name:    "success-once",
				Expires: time.Now().Add(-(time.Hour * 2)),
			})
		}
		c.Locals("Success", msg)

		return c.Next()
	}
}
