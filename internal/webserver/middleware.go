package webserver

import (
	"github.com/gofiber/fiber/v2"
	jwtware "github.com/gofiber/jwt/v3"
	"github.com/svera/coreander/v3/internal/webserver/infrastructure"
	"github.com/svera/coreander/v3/internal/webserver/jwtclaimsreader"
	"github.com/svera/coreander/v3/internal/webserver/model"
)

func requireAdminMiddleware(c *fiber.Ctx) error {
	session := c.Locals("Session").(model.User)

	if session.Role != model.RoleAdmin {
		return fiber.ErrForbidden
	}

	return c.Next()
}

func allowIfNotLoggedInMiddleware(jwtSecret []byte) func(*fiber.Ctx) error {
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

func alwaysRequireAuthenticationMiddleware(jwtSecret []byte, sender Sender) func(*fiber.Ctx) error {
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

func configurableAuthenticationMiddleware(jwtSecret []byte, sender Sender, requireAuth bool) func(*fiber.Ctx) error {
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
		"Lang":                   chooseBestLanguage(c, getSupportedLanguages()),
		"Title":                  "Login",
		"Version":                c.App().Config().AppName,
		"EmailSendingConfigured": emailSendingConfigured,
		"SupportedLanguages":     getSupportedLanguages(),
	}, "layout")
}
