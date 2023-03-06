package controller

import (
	"fmt"
	"net/mail"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/svera/coreander/internal/infrastructure"
	"github.com/svera/coreander/internal/jwtclaimsreader"
	"github.com/svera/coreander/internal/model"
)

type authRepository interface {
	CheckCredentials(email, password string) (model.User, error)
	FindByEmail(email string) (model.User, error)
	FindByRecoveryUuid(recoveryUuid string) (model.User, error)
	GenerateRecovery(email string) (model.User, error)
	ClearRecovery(email string) error
	Update(user model.User) error
}

type recoveryEmail interface {
	Send(address, body string) error
}

type Auth struct {
	repository        authRepository
	secret            []byte
	sender            recoveryEmail
	minPasswordLength int
	hostname          string
	port              string
}

type AuthConfig struct {
	Secret            []byte
	MinPasswordLength int
	Hostname          string
	Port              string
}

const (
	defaultHttpPort  = "80"
	defaultHttpsPort = "443"
)

func NewAuth(repository authRepository, sender recoveryEmail, cfg AuthConfig) *Auth {
	return &Auth{
		repository:        repository,
		secret:            cfg.Secret,
		sender:            sender,
		minPasswordLength: cfg.MinPasswordLength,
		hostname:          cfg.Hostname,
		port:              cfg.Port,
	}
}

func (a *Auth) Login(c *fiber.Ctx) error {
	session := jwtclaimsreader.SessionData(c)

	if session.Uuid != "" {
		return fiber.ErrForbidden
	}

	return c.Render("auth/login", fiber.Map{
		"Lang":    c.Params("lang"),
		"Title":   "Login",
		"Version": c.App().Config().AppName,
		"Session": session,
	}, "layout")
}

// Signs in a user and gives them a JWT.
func (a *Auth) SignIn(c *fiber.Ctx) error {
	var (
		user model.User
		err  error
	)

	session := jwtclaimsreader.SessionData(c)

	if session.Uuid != "" {
		return fiber.ErrForbidden
	}

	// If username or password are incorrect, do not allow access.
	user, err = a.repository.CheckCredentials(c.FormValue("email"), c.FormValue("password"))
	if err != nil {
		return fiber.ErrInternalServerError
	}

	if user.Email == "" {
		return c.Status(fiber.StatusUnauthorized).Render("auth/login", fiber.Map{
			"Lang":    c.Params("lang"),
			"Title":   "Login",
			"Message": "Wrong email or password",
			"Version": c.App().Config().AppName,
			"Session": session,
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
	session := jwtclaimsreader.SessionData(c)

	if _, ok := a.sender.(*infrastructure.NoEmail); ok {
		return fiber.ErrNotFound
	}

	if session.Uuid != "" {
		return fiber.ErrForbidden
	}

	return c.Render("auth/recover", fiber.Map{
		"Lang":    c.Params("lang"),
		"Title":   "Recover password",
		"Version": c.App().Config().AppName,
		"Session": session,
		"Errors":  map[string]string{},
	}, "layout")
}

func (a *Auth) Request(c *fiber.Ctx) error {
	session := jwtclaimsreader.SessionData(c)
	errs := map[string]string{}

	if _, ok := a.sender.(*infrastructure.NoEmail); ok {
		return fiber.ErrNotFound
	}

	if session.Uuid != "" {
		return fiber.ErrForbidden
	}

	if _, err := mail.ParseAddress(c.FormValue("email")); err != nil {
		errs["sendtoemail"] = "Incorrect send to email address"
	}

	if len(errs) > 1 {
		return c.Render("auth/recover", fiber.Map{
			"Lang":    c.Params("lang"),
			"Title":   "Recover password",
			"Version": c.App().Config().AppName,
			"Session": session,
			"Errors":  errs,
		}, "layout")
	}

	user, err := a.repository.GenerateRecovery(c.FormValue("email"))
	if err != nil {
		return fiber.ErrInternalServerError
	}

	port := ":" + a.port
	if (a.port == "80" && c.Protocol() == "http") ||
		(a.port == "443" && c.Protocol() == "https") {
		port = ""
	}
	if user.RecoveryUUID != "" {
		c.Render("auth/email", fiber.Map{
			"Lang":     c.Params("lang"),
			"Uuid":     user.RecoveryUUID,
			"Protocol": c.Protocol(),
			"Hostname": a.hostname,
			"Port":     port,
		})

		go a.sender.Send(c.FormValue("email"), string(c.Response().Body()))
	}

	return c.Render("auth/request", fiber.Map{
		"Lang":    c.Params("lang"),
		"Title":   "Recover password",
		"Version": c.App().Config().AppName,
		"Session": session,
		"Errors":  errs,
	}, "layout")
}

func (a *Auth) EditPassword(c *fiber.Ctx) error {
	_, err := a.validateRecoveryAccess(c, c.Query("id"))
	if err != nil {
		return err
	}

	return c.Render("auth/edit-password", fiber.Map{
		"Lang":    c.Params("lang"),
		"Title":   "Reset password",
		"Version": c.App().Config().AppName,
		"Session": model.User{},
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
	errs := map[string]string{}
	errs = user.ConfirmPassword(c.FormValue("confirm-password"), a.minPasswordLength, errs)
	if len(errs) > 0 {
		return c.Render("auth/edit-password", fiber.Map{
			"Lang":    c.Params("lang"),
			"Title":   "Reset password",
			"Session": model.User{},
			"Version": c.App().Config().AppName,
			"Uuid":    c.Query("id"),
			"Errors":  errs,
		}, "layout")
	}

	if err := a.repository.Update(user); err != nil {
		return fiber.ErrInternalServerError
	}

	if err := a.repository.ClearRecovery(user.Email); err != nil {
		return fiber.ErrInternalServerError
	}

	return c.Redirect(fmt.Sprintf("/%s/login", c.Params("lang")))
}

func (a *Auth) validateRecoveryAccess(c *fiber.Ctx, recoveryUuid string) (model.User, error) {
	session := jwtclaimsreader.SessionData(c)

	if _, ok := a.sender.(*infrastructure.NoEmail); ok {
		return session, fiber.ErrNotFound
	}

	if session.Uuid != "" {
		return session, fiber.ErrForbidden
	}

	if recoveryUuid == "" {
		return session, fiber.ErrBadRequest
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
