package user

import (
	"log"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

// ShareRecipients returns usernames and names for autocomplete.
func (u *Controller) ShareRecipients(c fiber.Ctx) error {
	query := strings.TrimSpace(c.Query("q"))
	users, err := u.usersRepository.UsernamesAndNames(query)
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	session, _ := c.Locals("Session").(model.Session)
	response := make([]fiber.Map, 0, len(users))
	for _, user := range users {
		if session.Username != "" && user.Username == session.Username {
			continue
		}
		response = append(response, fiber.Map{
			"username": user.Username,
			"name":     user.Name,
		})
	}

	return c.JSON(response)
}
