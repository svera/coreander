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

type Auth struct {
	repository *model.Auth
	version    string
}

func NewAuth(repository *model.Auth, version string) *Auth {
	return &Auth{
		repository: repository,
		version:    version,
	}
}

func (a *Auth) Login(c *fiber.Ctx) error {
	userData := jwtclaimsreader.UserData(c)

	return c.Render("login", fiber.Map{
		"Lang":     c.Params("lang"),
		"Title":    "Login",
		"Version":  a.version,
		"UserData": userData,
	}, "layout")
}

// Signs in a user and gives them a JWT.
func (a *Auth) SignIn(c *fiber.Ctx) error {
	var (
		user model.User
		err  error
	)

	// Create a struct so the request body can be mapped here.
	type loginRequest struct {
		Username string `form:"username"`
		Password string `form:"password"`
	}

	userData := jwtclaimsreader.UserData(c)

	// Get request body.
	request := &loginRequest{}
	if err := c.BodyParser(request); err != nil {
		return c.Status(fiber.StatusInternalServerError).Render("errors/internal", fiber.Map{
			"Lang":     c.Params("lang"),
			"Title":    "Login",
			"Version":  a.version,
			"UserData": userData,
		}, "layout")
	}

	// If username or password are incorrect, do not allow access.
	if user, err = a.repository.CheckCredentials(request.Username, request.Password); err != nil {
		return c.Status(fiber.StatusUnauthorized).Render("login", fiber.Map{
			"Lang":     c.Params("lang"),
			"Title":    "Login",
			"Message":  "Wrong username or password",
			"Version":  a.version,
			"UserData": userData,
		}, "layout")
	}

	// Send back JWT as a cookie.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userdata": model.UserData{
			Name:     user.Name,
			UserName: request.Username,
			Role:     user.Role,
			Uuid:     user.Uuid,
		},
		"exp": jwt.NewNumericDate(time.Now().Add(time.Hour * 24)),
	},
	)
	signedToken, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).Render("errors/internal", fiber.Map{
			"Lang":     c.Params("lang"),
			"Title":    "Login",
			"UserData": userData,
			"Version":  a.version,
		}, "layout")
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
