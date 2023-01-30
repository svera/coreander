package controller

import (
	"fmt"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/svera/coreander/internal/model"
)

type Auth struct {
	repository *model.Auth
}

func NewAuth(repository *model.Auth) *Auth {
	return &Auth{
		repository: repository,
	}
}

func (a *Auth) Login(c *fiber.Ctx) error {
	return c.Render("login", fiber.Map{
		"Lang":  c.Params("lang"),
		"Title": "Login",
	}, "layout")
}

// Signs in a user and gives them a JWT.
func (a *Auth) SignInUser(c *fiber.Ctx) error {
	// Create a struct so the request body can be mapped here.
	type loginRequest struct {
		Username string `form:"username"`
		Password string `form:"password"`
	}

	// Create a struct for our custom JWT payload.
	type jwtClaims struct {
		User string `form:"user"`
		jwt.StandardClaims
	}

	// Get request body.
	request := &loginRequest{}
	if err := c.BodyParser(request); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(&fiber.Map{
			"status":  "fail",
			"message": err.Error(),
		})
	}

	// If username or password are incorrect, do not allow access.

	if !a.repository.CheckCredentials(request.Username, request.Password) {
		return c.Status(fiber.StatusUnauthorized).JSON(&fiber.Map{
			"status":  "fail",
			"message": "Wrong username or password!",
		})
	}

	// Send back JWT as a cookie.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &jwtClaims{
		request.Username,
		jwt.StandardClaims{
			Audience:  "coreander-users",
			ExpiresAt: time.Now().Add(time.Hour * 24).Unix(),
			Issuer:    "coreander",
		},
	})
	signedToken, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(&fiber.Map{
			"status":  "fail",
			"message": err.Error(),
		})
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
func (a *Auth) SignOutUser(c *fiber.Ctx) error {
	c.Cookie(&fiber.Cookie{
		Name:     "jwt",
		Value:    "loggedOut",
		Path:     "/",
		Expires:  time.Now().Add(time.Second * 10),
		Secure:   false,
		HTTPOnly: true,
	})

	return c.Redirect(fmt.Sprintf("/%s", c.Params("lang")))
}
