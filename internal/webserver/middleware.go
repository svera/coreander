package webserver

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	jwtware "github.com/gofiber/jwt/v3"
	"github.com/svera/coreander/v3/internal/webserver/infrastructure"
	"github.com/svera/coreander/v3/internal/webserver/jwtclaimsreader"
	"github.com/svera/coreander/v3/internal/webserver/model"
)

// RequireAdmin returns HTTP forbidden if the user requesting access
// is not an admin
func RequireAdmin(c *fiber.Ctx) error {
	session := c.Locals("Session").(model.User)

	if session.Role != model.RoleAdmin {
		return fiber.ErrForbidden
	}

	return c.Next()
}

// SetFQDN composes the Fully Qualified Domain Name of the host running the app and sets it
// as a local variable of the request
func SetFQDN(cfg Config) func(*fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		c.Locals("fqdn", fmt.Sprintf("%s://%s%s",
			c.Protocol(),
			cfg.Hostname,
			urlPort(c.Protocol(), cfg.Port),
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
		TokenLookup:   "cookie:coreander",
		SuccessHandler: func(c *fiber.Ctx) error {
			return fiber.ErrForbidden
		},
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Next()
		},
	})
}

func AlwaysRequireAuthentication(jwtSecret []byte, sender Sender) func(*fiber.Ctx) error {
	return jwtware.New(jwtware.Config{
		SigningKey:    jwtSecret,
		SigningMethod: "HS256",
		TokenLookup:   "cookie:coreander",
		SuccessHandler: func(c *fiber.Ctx) error {
			c.Locals("Session", jwtclaimsreader.SessionData(c))
			return c.Next()
		},
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return forbidden(c, sender)
		},
	})
}

func ConfigurableAuthentication(jwtSecret []byte, sender Sender, requireAuth bool) func(*fiber.Ctx) error {
	return jwtware.New(jwtware.Config{
		SigningKey:    jwtSecret,
		SigningMethod: "HS256",
		TokenLookup:   "cookie:coreander",
		SuccessHandler: func(c *fiber.Ctx) error {
			c.Locals("Session", jwtclaimsreader.SessionData(c))
			return c.Next()
		},
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			err = c.Next()
			if requireAuth {
				return forbidden(c, sender)
			}
			return err
		},
	})
}

func forbidden(c *fiber.Ctx, sender Sender) error {
	emailSendingConfigured := true
	if _, ok := sender.(*infrastructure.NoEmail); ok {
		emailSendingConfigured = false
	}

	return c.Status(fiber.StatusForbidden).Render("auth/login", fiber.Map{
		"Lang":                   chooseBestLanguage(c),
		"Title":                  "Login",
		"Version":                c.App().Config().AppName,
		"EmailSendingConfigured": emailSendingConfigured,
	}, "layout")
}
