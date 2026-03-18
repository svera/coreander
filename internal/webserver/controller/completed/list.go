package completed

import (
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v4/internal/result"
	"github.com/svera/coreander/v4/internal/webserver/model"
	"github.com/svera/coreander/v4/internal/webserver/view"
)

// Completed renders the list of documents completed by the user
func (c *Controller) Completed(ctx fiber.Ctx) error {
	var session model.Session
	if val, ok := ctx.Locals("Session").(model.Session); ok {
		session = val
	}

	page, err := strconv.Atoi(ctx.Query("page"))
	if err != nil {
		page = 1
	}

	var year int
	if ctx.Query("year") == "" {
		year = time.Now().Year()
	} else {
		year, _ = strconv.Atoi(ctx.Query("year"))
	}

	sortBy := ctx.Query("sort-by")
	orderBy := "completed_on DESC" // "completed last" = most recently completed at top
	if sortBy == "completed-newest-first" {
		// "completed first" = items completed first in time (oldest) at top
		orderBy = "completed_on ASC"
	}

	var results result.Paginated[[]model.AugmentedDocument]
	var startDate, endDate *time.Time
	if year != 0 {
		s := time.Date(year, 1, 1, 0, 0, 0, 0, time.Local)
		e := time.Date(year, 12, 31, 23, 59, 59, 999999999, time.Local)
		startDate, endDate = &s, &e
	}
	results, err = c.readingRepository.CompletedPaginatedBetweenDates(int(session.User.ID), startDate, endDate, page, int(model.ResultsPerPage), orderBy)
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	yearStats, err := c.readingRepository.CompletedStatsByYear(int(session.User.ID), session.User.WordsPerMinute)
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	layout := "layout"
	if ctx.Query("view") == "list" {
		layout = ""
	}

	templateVars := fiber.Map{
		"User":                   &session.User,
		"Results":                results,
		"Paginator":               view.Pagination(model.MaxPagesNavigator, results, ctx.Queries()),
		"Title":                  "Completions",
		"URL":                    view.URL(ctx),
		"WordsPerMinute":         session.User.WordsPerMinute,
		"Year":                   year,
		"YearStats":              yearStats,
		"SortURL":                view.BaseURLWithout(ctx, "sort-by", "page"),
		"SortBy":                 ctx.Query("sort-by"),
		"AdditionalSortOptions": []struct {
			Key   string
			Value string
		}{
			{"completed-newest-first", "completed first"},
			{"completed-oldest-first", "completed last"},
		},
	}

	if ctx.Get("hx-request") == "true" {
		if err := ctx.Render("partials/completed-list", templateVars); err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
		return nil
	}

	if err := ctx.Render("completed/list", templateVars, layout); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	return nil
}
