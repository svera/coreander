package user

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
func (u *Controller) Completed(c fiber.Ctx) error {
	user, err := u.usersRepository.FindByUsername(c.Params("username"))
	if err != nil {
		log.Println(err.Error())
		return fiber.ErrInternalServerError
	}
	if user == nil {
		return fiber.ErrNotFound
	}

	var session model.Session
	if val, ok := c.Locals("Session").(model.Session); ok {
		session = val
	}

	if session.Role != model.RoleAdmin && session.Username != c.Params("username") {
		return fiber.ErrForbidden
	}

	page, err := strconv.Atoi(c.Query("page"))
	if err != nil {
		page = 1
	}

	var statsYear int
	if c.Query("stats-year") == "" {
		statsYear = time.Now().Year()
	} else {
		statsYear, _ = strconv.Atoi(c.Query("stats-year"))
	}

	var paginatedReadings result.Paginated[[]model.Reading]
	if statsYear == 0 {
		paginatedReadings, err = u.readingRepository.CompletedPaginated(int(user.ID), page, int(model.ResultsPerPage))
	} else {
		startOfYear := time.Date(statsYear, 1, 1, 0, 0, 0, 0, time.Local)
		endOfYear := time.Date(statsYear, 12, 31, 23, 59, 59, 999999999, time.Local)
		paginatedReadings, err = u.readingRepository.CompletedPaginatedBetweenDates(int(user.ID), &startOfYear, &endOfYear, page, int(model.ResultsPerPage))
	}
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	augmented := make([]model.AugmentedDocument, 0, len(paginatedReadings.Hits()))
	for _, reading := range paginatedReadings.Hits() {
		doc, err := u.indexer.Document(reading.Slug)
		if err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
		if doc.ID == "" {
			continue
		}
		augmented = append(augmented, model.AugmentedDocument{
			Document:    doc,
			CompletedOn: reading.CompletedOn,
		})
	}

	results := result.NewPaginated(
		int(model.ResultsPerPage),
		page,
		paginatedReadings.TotalHits(),
		augmented,
	)

	statsYearsRaw := u.readingStatsYears(user.ID)
	statsYears := append([]int{0}, statsYearsRaw...) // 0 = "All time"
	yearlyCompletedCount, yearlyReadingTime := u.calculateYearlyStats(int(user.ID), user.WordsPerMinute, statsYear)
	if statsYear == 0 {
		yearlyCompletedCount, yearlyReadingTime = u.calculateLifetimeStats(int(user.ID), user.WordsPerMinute)
	}

	layout := "layout"
	if c.Query("view") == "list" {
		layout = ""
	}

	templateVars := fiber.Map{
		"User":                   user,
		"Results":                results,
		"Paginator":               view.Pagination(model.MaxPagesNavigator, results, c.Queries()),
		"Title":                  "Completed documents",
		"URL":                    view.URL(c),
		"WordsPerMinute":         user.WordsPerMinute,
		"StatsYear":              statsYear,
		"StatsYears":             statsYears,
		"YearlyCompletedCount": yearlyCompletedCount,
		"YearlyReadingTime":    yearlyReadingTime,
	}

	if c.Get("hx-request") == "true" {
		if err := c.Render("partials/completed-list", templateVars); err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
		return nil
	}

	if err := c.Render("user/completed", templateVars, layout); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	return nil
}
