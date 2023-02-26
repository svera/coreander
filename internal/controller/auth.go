package controller

import (
	"fmt"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/svera/coreander/internal/jwtclaimsreader"
	"github.com/svera/coreander/internal/model"
)

type authRepository interface {
	CheckCredentials(email, password string) (model.User, error)
}

type Auth struct {
	repository authRepository
}

func NewAuth(repository authRepository) *Auth {
	return &Auth{
		repository: repository,
	}
}

func (a *Auth) Login(c *fiber.Ctx) error {
	session := jwtclaimsreader.SessionData(c)

	if session.Uuid != "" {
		return fiber.ErrForbidden
	}

	return c.Render("login", fiber.Map{
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
			Name:        user.Name,
			Email:       user.Email,
			Role:        user.Role,
			Uuid:        user.Uuid,
			SendToEmail: user.SendToEmail,
		},
		"exp": jwt.NewNumericDate(time.Now().Add(time.Hour * 24)),
	},
	)
	signedToken, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		return fiber.ErrInternalServerError
	}
	c.Cookie(&fiber.Cookie{
		Name:     "jwt",
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
		Name:     "jwt",
		Value:    "",
		Path:     "/",
		Expires:  time.Now().Add(-time.Second * 10),
		Secure:   false,
		HTTPOnly: true,
	})

	return c.Redirect(fmt.Sprintf("/%s", c.Params("lang")))
}
