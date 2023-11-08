package user

import (
	"math"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/model"
	"github.com/svera/coreander/v4/internal/webserver/controller"
	"github.com/svera/coreander/v4/internal/webserver/jwtclaimsreader"
)

// List list all users registered in the database
func (u *Controller) List(c *fiber.Ctx) error {
	session := jwtclaimsreader.SessionData(c)

	if session.Role != model.RoleAdmin {
		return fiber.ErrForbidden
	}

	page, err := strconv.Atoi(c.Query("page"))
	if err != nil {
		page = 1
	}
	totalRows := u.repository.Total()
	totalPages := int(math.Ceil(float64(totalRows) / model.ResultsPerPage))

	users, _ := u.repository.List(page, model.ResultsPerPage)
	return c.Render("users/index", fiber.Map{
		"Title":     "Users",
		"Users":     users,
		"Paginator": controller.Pagination(model.MaxPagesNavigator, totalPages, page, map[string]string{}),
		"Session":   session,
		"Admins":    u.repository.Admins(),
	}, "layout")
}
