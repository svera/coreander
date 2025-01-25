package user

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/webserver/model"
	"github.com/svera/coreander/v4/internal/webserver/view"
)

// List list all users registered in the database
func (u *Controller) List(c *fiber.Ctx) error {
	page, err := strconv.Atoi(c.Query("page"))
	if err != nil {
		page = 1
	}

	users, _ := u.repository.List(page, model.ResultsPerPage)

	templateVars := fiber.Map{
		"Title":     "Users",
		"Users":     users.Hits(),
		"Paginator": view.Pagination(model.MaxPagesNavigator, users, map[string]string{}),
		"Admins":    u.repository.Admins(),
		"URL":       view.URL(c),
	}

	if c.Get("hx-request") == "true" {
		return c.Render("partials/users-list", templateVars)
	}

	return c.Render("user/index", templateVars, "layout")
}
