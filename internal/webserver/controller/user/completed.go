package user

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v4/internal/metadata"
	"github.com/svera/coreander/v4/internal/result"
	"github.com/svera/coreander/v4/internal/webserver/model"
	"github.com/svera/coreander/v4/internal/webserver/view"
)

// Completed renders the list of documents completed by the user
func (u *Controller) Completed(c fiber.Ctx) error {
	var session model.Session
	if val, ok := c.Locals("Session").(model.Session); ok {
		session = val
	}

	if session.Username == "" {
		return fiber.ErrForbidden
	}
	user := &session.User

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

	sortBy := c.Query("sort-by")
	orderBy := "completed_on DESC" // "completed last" = most recently completed at top
	if sortBy == "completed-newest-first" {
		// "completed first" = items completed first in time (oldest) at top
		orderBy = "completed_on ASC"
	}

	var results result.Paginated[[]model.AugmentedDocument]
	sortByReadingTime := sortBy == "reading-time-shortest-first" || sortBy == "reading-time-longest-first"

	if sortByReadingTime {
		var startDate, endDate *time.Time
		if statsYear != 0 {
			s := time.Date(statsYear, 1, 1, 0, 0, 0, 0, time.Local)
			e := time.Date(statsYear, 12, 31, 23, 59, 59, 999999999, time.Local)
			startDate, endDate = &s, &e
		}
		results, err = u.readingRepository.CompletedPaginatedBetweenDatesByWords(int(user.ID), startDate, endDate, page, int(model.ResultsPerPage), sortBy == "reading-time-shortest-first")
	} else {
		if statsYear == 0 {
			results, err = u.readingRepository.CompletedPaginated(int(user.ID), page, int(model.ResultsPerPage), orderBy)
		} else {
			startOfYear := time.Date(statsYear, 1, 1, 0, 0, 0, 0, time.Local)
			endOfYear := time.Date(statsYear, 12, 31, 23, 59, 59, 999999999, time.Local)
			results, err = u.readingRepository.CompletedPaginatedBetweenDates(int(user.ID), &startOfYear, &endOfYear, page, int(model.ResultsPerPage), orderBy)
		}
	}
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	yearStats, err := u.completedYearStats(int(user.ID), user.WordsPerMinute)
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
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
		"Title":                  "Completions",
		"URL":                    view.URL(c),
		"WordsPerMinute":         user.WordsPerMinute,
		"StatsYear":              statsYear,
		"YearStats":              yearStats,
		"YearlyCompletedCount":   yearlyCompletedCount,
		"YearlyReadingTime":      yearlyReadingTime,
		"SortURL":                view.BaseURLWithout(c, "sort-by", "page"),
		"SortBy":                 c.Query("sort-by"),
		"AdditionalSortOptions": []struct {
			Key   string
			Value string
		}{
			{"completed-newest-first", "completed first"},
			{"completed-oldest-first", "completed last"},
			{"reading-time-shortest-first", "shortest first"},
			{"reading-time-longest-first", "longest first"},
		},
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

// completedYearStats returns year stats (including "All time" as year 0) with document count, words, and estimated reading time per year.
func (u *Controller) completedYearStats(userID int, wordsPerMinute float64) ([]model.CompletedYearStats, error) {
	rows, err := u.readingRepository.CompletedStatsByYear(userID)
	if err != nil {
		return nil, err
	}
	allSlugs, err := u.readingRepository.CompletedBetweenDates(userID, nil, nil)
	if err != nil {
		return nil, err
	}
	allWords, _ := u.indexer.TotalWordCount(allSlugs)
	stats := []model.CompletedYearStats{{
		Year:          0,
		DocumentCount: len(allSlugs),
		Words:         allWords,
		ReadingTime:   u.wordsToReadingTime(allWords, wordsPerMinute),
	}}
	for _, row := range rows {
		words, _ := u.indexer.TotalWordCount(row.Slugs)
		stats = append(stats, model.CompletedYearStats{
			Year:          row.Year,
			DocumentCount: row.DocumentCount,
			Words:         words,
			ReadingTime:   u.wordsToReadingTime(words, wordsPerMinute),
		})
	}
	return stats, nil
}

func (u *Controller) wordsToReadingTime(words float64, wordsPerMinute float64) string {
	if words <= 0 || wordsPerMinute <= 0 {
		return ""
	}
	if readingTime, err := time.ParseDuration(fmt.Sprintf("%fm", words/wordsPerMinute)); err == nil {
		return metadata.FmtDuration(readingTime)
	}
	return ""
}
