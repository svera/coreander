package webserver

import (
	"time"

	"github.com/form3tech-oss/jwt-go"
	"github.com/gofiber/fiber/v2"
)

func routeLogIn(c *fiber.Ctx, secret string) error {
	lang := c.Params("lang")
	if _, ok := languages[lang]; !ok {
		return fiber.ErrBadRequest
	}
	user := c.FormValue("email")
	pass := c.FormValue("password")

	// Throws Unauthorized error
	if user != "sergio.vera@gmail.com" || pass != "doe" {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	// Create token
	token := jwt.New(jwt.SigningMethodHS256)

	// Set claims
	claims := token.Claims.(jwt.MapClaims)
	claims["name"] = "John Doe"
	claims["admin"] = true
	claims["exp"] = time.Now().Add(time.Hour * 72).Unix()

	// Generate encoded token and send it as response.
	t, err := token.SignedString([]byte(secret))

	if err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	c.Cookie(&fiber.Cookie{
		Name:     "coreander",
		Value:    t,
		Expires:  time.Now().Add(time.Hour * 72),
		HTTPOnly: true,
	})
	return c.Redirect("/" + lang)
}
