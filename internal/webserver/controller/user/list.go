package user

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v3/internal/webserver/model"
	"github.com/svera/coreander/v3/internal/webserver/view"
)

// List list all users registered in the database
func (u *Controller) List(c *fiber.Ctx) error {
	page, err := strconv.Atoi(c.Query("page"))
	if err != nil {
		page = 1
	}

	users, _ := u.repository.List(page, model.ResultsPerPage)
	return c.Render("users/index", fiber.Map{
		"Title":     "Users",
		"Users":     users.Hits(),
		"Paginator": view.Pagination(model.MaxPagesNavigator, users, map[string]string{}),
		"Admins":    u.repository.Admins(),
	}, "layout")
}
