package user

import (
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

// ShareRecipients returns usernames and names for autocomplete.
func (u *Controller) ShareRecipients(c *fiber.Ctx) error {
	query := strings.TrimSpace(c.Query("q"))
	var (
		users []model.User
		err   error
	)
	if query == "" {
		users, err = u.usersRepository.UsernamesAndNames()
	} else {
		users, err = u.usersRepository.UsernamesAndNamesFiltered(query)
	}
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
