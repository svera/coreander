package auth

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v4"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

// Signs in a user and gives them a JWT.
func (a *Controller) SignIn(c fiber.Ctx) error {
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
			"Title":            "Login",
			"Error":            "Wrong email or password",
			"DisableLoginLink": true,
		}, "layout")
	}

	// Send back JWT as a cookie.
	expiration := time.Now().Add(a.config.SessionTimeout)
	signedToken, err := GenerateToken(c, user, expiration, a.config.Secret)
	if err != nil {
		return fiber.ErrInternalServerError
	}

	c.Cookie(&fiber.Cookie{
		Name:     "session",
		Value:    signedToken,
		Path:     "/",
		MaxAge:   34560000, // 400 days which is the life limit imposed by Chrome
		Secure:   false,
		HTTPOnly: true,
	})

	referer := string(c.RequestCtx().Referer())
	if referer != "" && !strings.Contains(referer, "/sessions") {
		return c.Redirect().To(referer)
	}

	return c.Redirect().To("/")
}

func GenerateToken(c fiber.Ctx, user *model.User, expiration time.Time, secret []byte) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userdata": model.User{
			ID:                user.ID,
			Name:              user.Name,
			Username:          user.Username,
			Email:             user.Email,
			Role:              user.Role,
			Uuid:              user.Uuid,
			SendToEmail:       user.SendToEmail,
			WordsPerMinute:    user.WordsPerMinute,
			ShowFileName:      user.ShowFileName,
			PrivateProfile:    user.PrivateProfile,
			PreferredEpubType: user.PreferredEpubType,
			DefaultAction:     user.DefaultAction,
		},
		"exp": jwt.NewNumericDate(expiration),
	},
	)

	return token.SignedString(secret)
}

// fiber:context-methods migrated
