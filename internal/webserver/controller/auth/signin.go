package auth

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/svera/coreander/v3/internal/webserver/model"
)

// Signs in a user and gives them a JWT.
func (a *Controller) SignIn(c *fiber.Ctx) error {
	var (
		user *model.User
		err  error
	)

	// If username or password are incorrect, do not allow access.
	user, err = a.repository.FindByEmail(c.FormValue("email"))
	if err != nil {
		return fiber.ErrInternalServerError
	}

	if user == nil || user.Password != model.Hash(c.FormValue("password")) {
		return c.Status(fiber.StatusUnauthorized).Render("auth/login", fiber.Map{
			"Title": "Login",
			"Error": "Wrong email or password",
		}, "layout")
	}

	// Send back JWT as a cookie.
	err = PersistAsCookie(c, user, a.config.SessionTimeout, a.config.Secret)
	if err != nil {
		return fiber.ErrInternalServerError
	}

	referer := string(c.Context().Referer())
	if referer != "" && !strings.HasSuffix(referer, "login") {
		return c.Redirect(referer)
	}

	return c.Redirect(fmt.Sprintf("/%s", c.Params("lang")))
}

func PersistAsCookie(c *fiber.Ctx, user *model.User, sessionTimeout time.Duration, secret []byte) error {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userdata": model.User{
			ID:             user.ID,
			Name:           user.Name,
			Username:       user.Username,
			Email:          user.Email,
			Role:           user.Role,
			Uuid:           user.Uuid,
			SendToEmail:    user.SendToEmail,
			WordsPerMinute: user.WordsPerMinute,
		},
		"exp": jwt.NewNumericDate(time.Now().Add(sessionTimeout)),
	},
	)

	signedToken, err := token.SignedString(secret)
	if err != nil {
		return fiber.ErrInternalServerError
	}
	c.Cookie(&fiber.Cookie{
		Name:     "coreander",
		Value:    signedToken,
		Path:     "/",
		Expires:  time.Now().Add(sessionTimeout),
		Secure:   false,
		HTTPOnly: true,
	})
	return nil
}
