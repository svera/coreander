package jwtclaimsreader

import (
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/svera/coreander/v3/internal/webserver/model"
)

func SessionData(c *fiber.Ctx) model.User {
	var user model.User
	if t, ok := c.Locals("user").(*jwt.Token); ok {
		claims := t.Claims.(jwt.MapClaims)
		userDataMap := claims["userdata"].(map[string]interface{})
		if value, ok := userDataMap["ID"].(float64); ok {
			user.ID = uint(value)
		}
		if value, ok := userDataMap["Name"].(string); ok {
			user.Name = value
		}
		if value, ok := userDataMap["Username"].(string); ok {
			user.Email = value
		}
		if value, ok := userDataMap["Role"].(float64); ok {
			user.Role = int(value)
		}
		if value, ok := userDataMap["Uuid"].(string); ok {
			user.Uuid = value
		}
		if value, ok := userDataMap["SendToEmail"].(string); ok {
			user.SendToEmail = value
		}
		if value, ok := userDataMap["WordsPerMinute"].(float64); ok {
			user.WordsPerMinute = value
		}
	}

	return user
}
