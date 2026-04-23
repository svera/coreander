package user

import (
	"log"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"github.com/svera/coreander/v4/internal/webserver/model"
	"github.com/svera/coreander/v4/internal/webserver/view"
)

// buildUserListVars returns template data for the user list page (full or HTMX partial).
func (u *Controller) buildUserListVars(c fiber.Ctx, page int, filter string) fiber.Map {
	users, _ := u.usersRepository.List(page, model.ResultsPerPage, filter)
	_, noEmail := u.sender.(*infrastructure.NoEmail)
	return fiber.Map{
		"Title":                    "Users",
		"Users":                    users.Hits(),
		"Paginator":                view.Pagination(model.MaxPagesNavigator, users, c.Queries()),
		"Admins":                   u.usersRepository.Admins(),
		"URL":                      view.URL(c),
		"Filter":                   filter,
		"EmailConfigured":          !noEmail,
		"InviteEmailListMaxLength": u.config.InviteEmailListMaxLength,
		"InviteFormErrors":         map[string]string{},
		"InviteFormEmail":          "",
		"InviteFormOpen":           false,
	}
}

// List list all users registered in the database
func (u *Controller) List(c fiber.Ctx) error {
	page, err := strconv.Atoi(c.Query("page"))
	if err != nil {
		page = 1
	}

	filter := c.Query("filter", "")

	templateVars := u.buildUserListVars(c, page, filter)

	if c.Get("hx-request") == "true" {
		// Render table rows and pagination update in one response
		// HTMX will extract the out-of-band swap element before swapping rows into tbody
		if err = c.Render("partials/users-table-response", templateVars); err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
		return nil
	}

	return c.Render("user/list", templateVars, "layout")
}
