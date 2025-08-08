package user

import (
	"strconv"
	"time"

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

	msg := ""
	if c.Cookies("success") == "true" {
		c.Cookie(&fiber.Cookie{
			Name:    "success",
			Expires: time.Now().Add(-(time.Hour * 2)),
		})
		msg = "User created."
	}

	templateVars := fiber.Map{
		"Title":     "Users",
		"Users":     users.Hits(),
		"Paginator": view.Pagination(model.MaxPagesNavigator, users, c.Queries()),
		"Admins":    u.repository.Admins(),
		"URL":       view.URL(c),
		"Message":   msg,
	}

	return c.Render("user/index", templateVars, "layout")
}
