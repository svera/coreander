package user

import (
	"log"

	"github.com/gofiber/fiber/v2"
)

// Usernames returns all usernames as JSON for autocomplete.
func (u *Controller) Usernames(c *fiber.Ctx) error {
	usernames, err := u.usersRepository.Usernames()
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	return c.JSON(usernames)
}
