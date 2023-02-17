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
	session := jwtclaimsreader.SessionData(c)

	return c.Render("login", fiber.Map{
		"Lang":    c.Params("lang"),
		"Title":   "Login",
		"Version": a.version,
		"Session": session,
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
		Email    string `form:"email"`
		Password string `form:"password"`
	}

	session := jwtclaimsreader.SessionData(c)

	// Get request body.
	request := &loginRequest{}
	if err := c.BodyParser(request); err != nil {
		return c.Status(fiber.StatusInternalServerError).Render("errors/internal", fiber.Map{
			"Lang":    c.Params("lang"),
			"Title":   "Login",
			"Version": a.version,
			"Session": session,
		}, "layout")
	}

	// If username or password are incorrect, do not allow access.
	if user, err = a.repository.CheckCredentials(request.Email, request.Password); err != nil {
		return c.Status(fiber.StatusUnauthorized).Render("login", fiber.Map{
			"Lang":    c.Params("lang"),
			"Title":   "Login",
			"Message": "Wrong email or password",
			"Version": a.version,
			"Session": session,
		}, "layout")
	}

	// Send back JWT as a cookie.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userdata": model.UserData{
			Name:  user.Name,
			Email: request.Email,
			Role:  user.Role,
			Uuid:  user.Uuid,
		},
		"exp": jwt.NewNumericDate(time.Now().Add(time.Hour * 24)),
	},
	)
	signedToken, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).Render("errors/internal", fiber.Map{
			"Lang":    c.Params("lang"),
			"Title":   "Login",
			"Session": session,
			"Version": a.version,
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
