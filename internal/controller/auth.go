package controller

import (
	"fmt"
	"log"
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
	var (
		user model.User
		err  error
	)

	// Create a struct so the request body can be mapped here.
	type loginRequest struct {
		Username string `form:"username"`
		Password string `form:"password"`
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
	if user, err = a.repository.CheckCredentials(request.Username, request.Password); err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(&fiber.Map{
			"status":  "fail",
			"message": "Wrong username or password!",
		})
	}

	// Send back JWT as a cookie.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"name":     user.Name,
		"username": request.Username,
		"role":     user.Role,
		"exp":      jwt.NewNumericDate(time.Now().Add(time.Hour * 24)),
	},
	)
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
	c.Append("Cache-Time", "0")

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

func getJWTClaimsFromCookie(c *fiber.Ctx) (jwt.MapClaims, error) {
	var (
		token *jwt.Token
		err   error
	)

	cookie := c.Cookies("jwt")
	claims := jwt.MapClaims{}
	if cookie != "" {
		token, err = jwt.ParseWithClaims(cookie, &claims, func(token *jwt.Token) (interface{}, error) {
			return []byte(os.Getenv("JWT_SECRET")), nil
		})
		if err != nil {
			log.Println(err)
			return claims, err
		}

		if err = token.Claims.Valid(); err != nil {
			return claims, err
		}
		return claims, nil
	}

	return claims, fmt.Errorf("cookie not available")
}
