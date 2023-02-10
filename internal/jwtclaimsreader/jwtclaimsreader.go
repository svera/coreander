package jwtclaimsreader

import (
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/svera/coreander/internal/model"
)

type Config struct {
	Filter func(c *fiber.Ctx) bool
}

func New(config Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if config.Filter != nil && config.Filter(c) {
			return c.Next()
		}

		var (
			token  *jwt.Token
			err    error
			claims jwt.MapClaims
		)

		cookie := c.Cookies("jwt")
		claims = jwt.MapClaims{
			"userdata": model.UserData{
				Role: model.RoleRegular,
			},
		}
		if cookie != "" {
			token, err = jwt.ParseWithClaims(cookie, &claims, func(token *jwt.Token) (interface{}, error) {
				return []byte(os.Getenv("JWT_SECRET")), nil
			})
			if err != nil {
				return c.Next()
			}

			if err = token.Claims.Valid(); err != nil {
				return c.Next()
			}
			c.Locals("claims", claims)
		}

		return c.Next()
	}
}

func UserData(c *fiber.Ctx) model.UserData {
	var userData model.UserData
	if claims, ok := c.Locals("claims").(jwt.MapClaims); ok {
		userDataMap := claims["userdata"].(map[string]interface{})
		if value, ok := userDataMap["Name"].(string); ok {
			userData.Name = value
		}
		if value, ok := userDataMap["Username"].(string); ok {
			userData.UserName = value
		}
		if value, ok := userDataMap["Role"].(float64); ok {
			userData.Role = value
		}
		if value, ok := userDataMap["Uuid"].(string); ok {
			userData.Uuid = value
		}
	}

	return userData
}
