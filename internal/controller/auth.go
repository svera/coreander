package controller

import (
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/svera/coreander/internal/infrastructure"
	"github.com/svera/coreander/internal/model"
	"golang.org/x/text/message"
)

type authRepository interface {
	FindByEmail(email string) (model.User, error)
	FindByRecoveryUuid(recoveryUuid string) (model.User, error)
	Update(user model.User) error
}

type recoveryEmail interface {
	Send(address, subject, body string) error
}

type Auth struct {
	repository        authRepository
	secret            []byte
	sender            recoveryEmail
	minPasswordLength int
	hostname          string
	port              int
	printers          map[string]*message.Printer
}

type AuthConfig struct {
	Secret            []byte
	MinPasswordLength int
	Hostname          string
	Port              int
}

const (
	defaultHttpPort  = 80
	defaultHttpsPort = 443
)

func NewAuth(repository authRepository, sender recoveryEmail, cfg AuthConfig, printers map[string]*message.Printer) *Auth {
	return &Auth{
		repository:        repository,
		secret:            cfg.Secret,
		sender:            sender,
		minPasswordLength: cfg.MinPasswordLength,
		hostname:          cfg.Hostname,
		port:              cfg.Port,
		printers:          printers,
	}
}

func (a *Auth) Login(c *fiber.Ctx) error {
	resetPassword := fmt.Sprintf(
		"%s://%s%s/%s/reset-password",
		c.Protocol(),
		a.hostname,
		a.urlPort(c),
		c.Params("lang"),
	)

	msg := ""
	if ref := string(c.Request().Header.Referer()); strings.HasPrefix(ref, resetPassword) {
		msg = "Password changed successfully. Please sign in."
	}

	return c.Render("auth/login", fiber.Map{
		"Lang":    c.Params("lang"),
		"Title":   "Login",
		"Version": c.App().Config().AppName,
		"Message": msg,
	}, "layout")
}

// Signs in a user and gives them a JWT.
func (a *Auth) SignIn(c *fiber.Ctx) error {
	var (
		user model.User
		err  error
	)

	// If username or password are incorrect, do not allow access.
	user, err = a.repository.FindByEmail(c.FormValue("email"))
	if err != nil {
		return fiber.ErrInternalServerError
	}

	if user.Password != model.Hash(c.FormValue("password")) {
		return c.Status(fiber.StatusUnauthorized).Render("auth/login", fiber.Map{
			"Lang":    c.Params("lang"),
			"Title":   "Login",
			"Error":   "Wrong email or password",
			"Version": c.App().Config().AppName,
		}, "layout")
	}

	// Send back JWT as a cookie.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userdata": model.User{
			Name:           user.Name,
			Email:          user.Email,
			Role:           user.Role,
			Uuid:           user.Uuid,
			SendToEmail:    user.SendToEmail,
			WordsPerMinute: user.WordsPerMinute,
		},
		"exp": jwt.NewNumericDate(time.Now().Add(time.Hour * 24)),
	},
	)

	signedToken, err := token.SignedString(a.secret)
	if err != nil {
		return fiber.ErrInternalServerError
	}
	c.Cookie(&fiber.Cookie{
		Name:     "coreander",
		Value:    signedToken,
		Path:     "/",
		Expires:  time.Now().Add(time.Hour * 24),
		Secure:   false,
		HTTPOnly: true,
	})

	return c.Redirect(fmt.Sprintf("/%s", c.Params("lang")))
}

// Logs out user and removes their JWT.
func (a *Auth) SignOut(c *fiber.Ctx) error {
	c.Cookie(&fiber.Cookie{
		Name:     "coreander",
		Value:    "",
		Path:     "/",
		Expires:  time.Now().Add(-time.Second * 10),
		Secure:   false,
		HTTPOnly: true,
	})

	return c.Redirect(fmt.Sprintf("/%s", c.Params("lang")))
}

func (a *Auth) Recover(c *fiber.Ctx) error {
	if _, ok := a.sender.(*infrastructure.NoEmail); ok {
		return fiber.ErrNotFound
	}

	return c.Render("auth/recover", fiber.Map{
		"Lang":    c.Params("lang"),
		"Title":   "Recover password",
		"Version": c.App().Config().AppName,
		"Errors":  map[string]string{},
	}, "layout")
}

func (a *Auth) Request(c *fiber.Ctx) error {
	if _, ok := a.sender.(*infrastructure.NoEmail); ok {
		return fiber.ErrNotFound
	}

	if _, err := mail.ParseAddress(c.FormValue("email")); err != nil {
		return c.Render("auth/recover", fiber.Map{
			"Lang":    c.Params("lang"),
			"Title":   "Recover password",
			"Version": c.App().Config().AppName,
			"Errors":  map[string]string{"email": "Incorrect email address"},
		}, "layout")
	}

	if user, err := a.repository.FindByEmail(c.FormValue("email")); err == nil {
		user.RecoveryUUID = uuid.NewString()
		user.RecoveryValidUntil = time.Now().Add(24 * time.Hour)
		if err := a.repository.Update(user); err != nil {
			return fiber.ErrInternalServerError
		}

		recoveryLink := fmt.Sprintf(
			"%s://%s%s/%s/reset-password?id=%s",
			c.Protocol(),
			a.hostname,
			a.urlPort(c),
			c.Params("lang"),
			user.RecoveryUUID,
		)
		c.Render("auth/email", fiber.Map{
			"Lang":         c.Params("lang"),
			"RecoveryLink": recoveryLink,
		})

		go a.sender.Send(
			c.FormValue("email"),
			a.printers[c.Params("lang")].Sprintf("Password recovery request"),
			string(c.Response().Body()),
		)
	}

	return c.Render("auth/request", fiber.Map{
		"Lang":    c.Params("lang"),
		"Title":   "Recover password",
		"Version": c.App().Config().AppName,
		"Errors":  map[string]string{},
	}, "layout")
}

func (a *Auth) EditPassword(c *fiber.Ctx) error {
	if _, err := a.validateRecoveryAccess(c, c.Query("id")); err != nil {
		return err
	}

	return c.Render("auth/edit-password", fiber.Map{
		"Lang":    c.Params("lang"),
		"Title":   "Reset password",
		"Version": c.App().Config().AppName,
		"Uuid":    c.Query("id"),
		"Errors":  map[string]string{},
	}, "layout")
}

func (a *Auth) UpdatePassword(c *fiber.Ctx) error {
	user, err := a.validateRecoveryAccess(c, c.FormValue("id"))
	if err != nil {
		return err
	}

	user.Password = c.FormValue("password")
	user.RecoveryUUID = ""
	user.RecoveryValidUntil = time.Unix(0, 0)
	errs := map[string]string{}

	if errs = user.ConfirmPassword(c.FormValue("confirm-password"), a.minPasswordLength, errs); len(errs) > 0 {
		return c.Render("auth/edit-password", fiber.Map{
			"Lang":    c.Params("lang"),
			"Title":   "Reset password",
			"Version": c.App().Config().AppName,
			"Uuid":    c.FormValue("id"),
			"Errors":  errs,
		}, "layout")
	}

	user.Password = model.Hash(user.Password)
	if err := a.repository.Update(user); err != nil {
		return fiber.ErrInternalServerError
	}

	return c.Redirect(fmt.Sprintf("/%s/login", c.Params("lang")))
}

func (a *Auth) validateRecoveryAccess(c *fiber.Ctx, recoveryUuid string) (model.User, error) {
	if _, ok := a.sender.(*infrastructure.NoEmail); ok {
		return model.User{}, fiber.ErrNotFound
	}

	if recoveryUuid == "" {
		return model.User{}, fiber.ErrBadRequest
	}
	user, err := a.repository.FindByRecoveryUuid(recoveryUuid)
	if err != nil {
		return user, fiber.ErrInternalServerError
	}

	if user.RecoveryValidUntil.Before(time.Now()) {
		return user, fiber.ErrBadRequest
	}

	return user, nil
}

func (a *Auth) urlPort(c *fiber.Ctx) string {
	port := fmt.Sprintf(":%d", a.port)
	if (a.port == defaultHttpPort && c.Protocol() == "http") ||
		(a.port == defaultHttpsPort && c.Protocol() == "https") {
		port = ""
	}
	return port
}
