package jwtclaimsreader

import (
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/svera/coreander/internal/model"
)

func SessionData(c *fiber.Ctx) model.UserData {
	var userData model.UserData
	if t, ok := c.Locals("user").(*jwt.Token); ok {
		claims := t.Claims.(jwt.MapClaims)
		userDataMap := claims["userdata"].(map[string]interface{})
		if value, ok := userDataMap["Name"].(string); ok {
			userData.Name = value
		}
		if value, ok := userDataMap["Username"].(string); ok {
			userData.Email = value
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
