package webserver

import (
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

func sessionData(c *fiber.Ctx) model.Session {
	var session model.Session

	if t, ok := c.Locals("user").(*jwt.Token); ok {
		claims := t.Claims.(jwt.MapClaims)
		userDataMap := claims["userdata"].(map[string]any)
		if value, ok := userDataMap["ID"].(float64); ok {
			session.ID = uint(value)
		}
		if value, ok := userDataMap["Name"].(string); ok {
			session.Name = value
		}
		if value, ok := userDataMap["Username"].(string); ok {
			session.Username = value
		}
		if value, ok := userDataMap["Email"].(string); ok {
			session.Email = value
		}
		if value, ok := userDataMap["Role"].(float64); ok {
			session.Role = int(value)
		}
		if value, ok := userDataMap["Uuid"].(string); ok {
			session.Uuid = value
		}
		if value, ok := userDataMap["SendToEmail"].(string); ok {
			session.SendToEmail = value
		}
		if value, ok := userDataMap["WordsPerMinute"].(float64); ok {
			session.WordsPerMinute = value
		}
		if value, ok := userDataMap["ShowFileName"].(bool); ok {
			session.ShowFileName = value
		}
		if value, ok := userDataMap["PrivateProfile"].(float64); ok {
			session.PrivateProfile = int(value)
		}
		if value, ok := userDataMap["PreferredEpubType"].(string); ok {
			session.PreferredEpubType = value
		}
		if value, ok := userDataMap["DefaultAction"].(string); ok {
			session.DefaultAction = value
		}

		session.Exp = claims["exp"].(float64)
	}

	return session
}
