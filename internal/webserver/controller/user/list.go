package user

import (
	"log"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"github.com/svera/coreander/v4/internal/webserver/model"
	"github.com/svera/coreander/v4/internal/webserver/view"
)

// List list all users registered in the database
func (u *Controller) List(c *fiber.Ctx) error {
	page, err := strconv.Atoi(c.Query("page"))
	if err != nil {
		page = 1
	}

	users, _ := u.usersRepository.List(page, model.ResultsPerPage)

	_, emailConfigured := u.sender.(*infrastructure.NoEmail)

	templateVars := fiber.Map{
		"Title":              "Users",
		"Users":              users.Hits(),
		"Paginator":          view.Pagination(model.MaxPagesNavigator, users, c.Queries()),
		"Admins":             u.usersRepository.Admins(),
		"URL":                view.URL(c),
		"EmailConfigured":    !emailConfigured,
		"AvailableLanguages": c.Locals("AvailableLanguages"),
	}

	if c.Get("hx-request") == "true" {
		if err = c.Render("partials/users-list", templateVars); err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
		return nil
	}

	return c.Render("user/index", templateVars, "layout")
}
