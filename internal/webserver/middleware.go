package webserver

import (
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	jwtware "github.com/gofiber/jwt/v3"
	"github.com/svera/coreander/v4/internal/i18n"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"github.com/svera/coreander/v4/internal/webserver/model"
	"golang.org/x/exp/slices"
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
func AlwaysRequireAuthentication(jwtSecret []byte, sender Sender, translator i18n.Translator, usersRepository *model.UserRepository) func(*fiber.Ctx) error {
	return jwtware.New(jwtware.Config{
		SigningKey:    jwtSecret,
		SigningMethod: "HS256",
		TokenLookup:   "cookie:session",
		SuccessHandler: func(c *fiber.Ctx) error {
			session := sessionData(c)
			c.Locals("Session", session)
			usersRepository.UpdateLastRequest(session.ID)
			updateUserLanguage(c, usersRepository, session)
			return c.Next()
		},
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return forbidden(c, sender, translator, err)
		},
	})
}

// ConfigurableAuthentication allows to enable or disable authentication on routes which may or may not require it
func ConfigurableAuthentication(jwtSecret []byte, sender Sender, translator i18n.Translator, requireAuth bool, usersRepository *model.UserRepository) func(*fiber.Ctx) error {
	return jwtware.New(jwtware.Config{
		SigningKey:    jwtSecret,
		SigningMethod: "HS256",
		TokenLookup:   "cookie:session",
		SuccessHandler: func(c *fiber.Ctx) error {
			session := sessionData(c)
			c.Locals("Session", session)
			usersRepository.UpdateLastRequest(session.ID)
			updateUserLanguage(c, usersRepository, session)
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

// SetAvailableLanguages retrieves available languages from the index and sets them
// as a local variable for use in templates
func SetAvailableLanguages(idx ProgressInfo) func(*fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		availableLanguages, err := idx.Languages()
		if err != nil {
			fmt.Println(err)
			availableLanguages = []string{}
		}
		c.Locals("AvailableLanguages", availableLanguages)
		return c.Next()
	}
}

// updateUserLanguage updates the authenticated user's language preference in the database
// when the language query parameter (?l=) is present
func updateUserLanguage(c *fiber.Ctx, usersRepository *model.UserRepository, session model.Session) {
	// Early return if language query parameter is not present
	lang := c.Query("l")
	if lang == "" {
		return
	}

	// Early return if language is not supported
	if !slices.Contains(getSupportedLanguages(), lang) {
		return
	}

	// Get user and update language preference
	user, err := usersRepository.FindByUuid(session.Uuid)
	if err != nil || user == nil {
		return
	}

	// Only update if language has changed
	if user.Language == lang {
		return
	}

	// Update language field explicitly using Model().Update() to ensure it's saved
	result := usersRepository.DB.Model(user).Update("language", lang)
	if result.Error != nil {
		log.Printf("error updating user language for user %s: %v\n", session.Uuid, result.Error)
		// Continue anyway, don't fail the request
	}
}

