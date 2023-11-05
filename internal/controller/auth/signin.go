package auth

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/svera/coreander/v4/internal/model"
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
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userdata": model.User{
			ID:             user.ID,
			Name:           user.Name,
			Email:          user.Email,
			Role:           user.Role,
			Uuid:           user.Uuid,
			SendToEmail:    user.SendToEmail,
			WordsPerMinute: user.WordsPerMinute,
		},
		"exp": jwt.NewNumericDate(time.Now().Add(a.config.SessionTimeout)),
	},
	)

	signedToken, err := token.SignedString(a.config.Secret)
	if err != nil {
		return fiber.ErrInternalServerError
	}
	c.Cookie(&fiber.Cookie{
		Name:     "coreander",
		Value:    signedToken,
		Path:     "/",
		Expires:  time.Now().Add(a.config.SessionTimeout),
		Secure:   false,
		HTTPOnly: true,
	})

	return c.Redirect(fmt.Sprintf("/%s", c.Params("lang")))
}
