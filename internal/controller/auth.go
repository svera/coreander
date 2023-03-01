package controller

import (
	"fmt"
	"net/mail"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/svera/coreander/internal/jwtclaimsreader"
	"github.com/svera/coreander/internal/model"
)

type authRepository interface {
	CheckCredentials(email, password string) (model.User, error)
	FindByEmail(email string) (model.User, error)
}

type Auth struct {
	repository             authRepository
	secret                 []byte
	emailSendingConfigured bool
}

func NewAuth(repository authRepository, secret []byte, emailSendingCOnfigured bool) *Auth {
	return &Auth{
		repository:             repository,
		secret:                 secret,
		emailSendingConfigured: emailSendingCOnfigured,
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
	if user, err = a.repository.CheckCredentials(c.FormValue("email"), c.FormValue("password")); err != nil {
		return c.Status(fiber.StatusUnauthorized).Render("login", fiber.Map{
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

	if !a.emailSendingConfigured {
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

	if !a.emailSendingConfigured {
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

	if user, _ := a.repository.FindByEmail(c.FormValue("email")); user.Email != "" {

	}

	return c.Render("auth/request", fiber.Map{
		"Lang":    c.Params("lang"),
		"Title":   "Recover password",
		"Version": c.App().Config().AppName,
		"Session": session,
		"Errors":  errs,
	}, "layout")
}
