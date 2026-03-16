package user

import (
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v4/internal/metadata"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

// Edit renders the edit user form
func (u *Controller) Edit(c fiber.Ctx) error {
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

	vars := fiber.Map{
		"Title":              "Edit user",
		"User":               user,
		"MinPasswordLength":  u.config.MinPasswordLength,
		"UsernamePattern":    model.UsernamePattern,
		"Errors":             map[string]string{},
		"EmailFrom":          u.sender.From(),
		"ActiveTab":          "options",
		"AvailableLanguages": c.Locals("AvailableLanguages"),
	}

	if c.Get("HX-Request") == "true" {
		return c.Render("user/edit", vars)
	}

	return c.Render("user/edit", vars, "layout")
}

func (u *Controller) calculateYearlyStats(userID int, wordsPerMinute float64, year int) (int, string) {
	now := time.Now()
	startOfYear := time.Date(year, 1, 1, 0, 0, 0, 0, now.Location())
	endOfYear := time.Date(year, 12, 31, 23, 59, 59, 999999999, now.Location())

	completedSlugs, err := u.readingRepository.CompletedBetweenDates(userID, &startOfYear, &endOfYear)
	if err != nil {
		log.Printf("error getting completed readings for user %d: %s\n", userID, err)
		return 0, ""
	}

	return u.calculateReadingStats(completedSlugs, userID, wordsPerMinute)
}

func (u *Controller) readingStatsYear(requestedYear int) int {
	nowYear := time.Now().Year()
	if requestedYear > 0 {
		return requestedYear
	}
	if requestedYear == 0 {
		return 0 // "All time"
	}
	return nowYear
}

func (u *Controller) readingStatsYears(userID uint) []int {
	nowYear := time.Now().Year()
	availableYears, err := u.readingRepository.CompletedYears(userID)
	if err != nil {
		log.Printf("error getting completed years for user %d: %s\n", userID, err)
		availableYears = []int{}
	}

	availableYears = append(availableYears, nowYear)
	sort.Slice(availableYears, func(i, j int) bool {
		return availableYears[i] > availableYears[j]
	})

	return availableYears
}

func (u *Controller) calculateLifetimeStats(userID int, wordsPerMinute float64) (int, string) {
	// Get all completed documents (no date filtering)
	completedSlugs, err := u.readingRepository.CompletedBetweenDates(userID, nil, nil)
	if err != nil {
		log.Printf("error getting lifetime completed readings for user %d: %s\n", userID, err)
		return 0, ""
	}

	return u.calculateReadingStats(completedSlugs, userID, wordsPerMinute)
}

func (u *Controller) calculateReadingStats(completedSlugs []string, userID int, wordsPerMinute float64) (int, string) {
	if len(completedSlugs) == 0 {
		return 0, ""
	}

	totalWords, err := u.indexer.TotalWordCount(completedSlugs)
	if err != nil {
		log.Printf("error getting total word count for user %d: %s\n", userID, err)
		return len(completedSlugs), ""
	}
	if totalWords == 0 {
		return len(completedSlugs), ""
	}

	// Calculate reading time and format it
	if readingTime, err := time.ParseDuration(fmt.Sprintf("%fm", totalWords/wordsPerMinute)); err == nil {
		return len(completedSlugs), metadata.FmtDuration(readingTime)
	}

	return len(completedSlugs), ""
}
