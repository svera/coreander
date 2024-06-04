package webserver

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	jwtware "github.com/gofiber/jwt/v3"
	"github.com/svera/coreander/v3/internal/webserver/infrastructure"
	"github.com/svera/coreander/v3/internal/webserver/model"
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
		TokenLookup:   "cookie:coreander",
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
func AlwaysRequireAuthentication(jwtSecret []byte, sender Sender) func(*fiber.Ctx) error {
	return jwtware.New(jwtware.Config{
		SigningKey:    jwtSecret,
		SigningMethod: "HS256",
		TokenLookup:   "cookie:coreander",
		SuccessHandler: func(c *fiber.Ctx) error {
			c.Locals("Session", sessionData(c))
			return c.Next()
		},
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return forbidden(c, sender, err)
		},
	})
}

// ConfigurableAuthentication allows to enable or disable authentication on routes which may or may not require it
func ConfigurableAuthentication(jwtSecret []byte, sender Sender, requireAuth bool) func(*fiber.Ctx) error {
	return jwtware.New(jwtware.Config{
		SigningKey:    jwtSecret,
		SigningMethod: "HS256",
		TokenLookup:   "cookie:coreander",
		SuccessHandler: func(c *fiber.Ctx) error {
			c.Locals("Session", sessionData(c))
			return c.Next()
		},
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			if requireAuth {
				return forbidden(c, sender, err)
			}
			return c.Next()
		},
	})
}

func forbidden(c *fiber.Ctx, sender Sender, err error) error {
	emailSendingConfigured := true
	if _, ok := sender.(*infrastructure.NoEmail); ok {
		emailSendingConfigured = false
	}
	message := ""
	if err.Error() != "missing or malformed JWT" && c.Cookies("coreander") != "void" {
		message = "Session expired, please log in again."
	}
	return c.Status(fiber.StatusForbidden).Render("auth/login", fiber.Map{
		"Lang":                   chooseBestLanguage(c),
		"Title":                  "Login",
		"Version":                c.App().Config().AppName,
		"EmailSendingConfigured": emailSendingConfigured,
		"DisableLoginLink":       true,
		"Warning":                message,
	}, "layout")
}
