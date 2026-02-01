package user

import (
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/metadata"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

// Edit renders the edit user form
func (u *Controller) Edit(c *fiber.Ctx) error {
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

	// Calculate yearly reading statistics
	yearlyCompletedCount, yearlyReadingTime := u.calculateYearlyStats(int(user.ID), user.WordsPerMinute)

	// Calculate lifetime reading statistics
	lifetimeCompletedCount, lifetimeReadingTime := u.calculateLifetimeStats(int(user.ID), user.WordsPerMinute)

	emailSendingConfigured := true
	if _, ok := u.sender.(*infrastructure.NoEmail); ok {
		emailSendingConfigured = false
	}

	return c.Render("user/edit", fiber.Map{
		"Title":                  "Edit user",
		"User":                   user,
		"MinPasswordLength":      u.config.MinPasswordLength,
		"UsernamePattern":        model.UsernamePattern,
		"Errors":                 map[string]string{},
		"EmailFrom":              u.sender.From(),
		"EmailSendingConfigured": emailSendingConfigured,
		"ActiveTab":              "options",
		"YearlyCompletedCount":   yearlyCompletedCount,
		"YearlyReadingTime":      yearlyReadingTime,
		"LifetimeCompletedCount": lifetimeCompletedCount,
		"LifetimeReadingTime":    lifetimeReadingTime,
		"AvailableLanguages":     c.Locals("AvailableLanguages"),
	}, "layout")
}

func (u *Controller) calculateYearlyStats(userID int, wordsPerMinute float64) (int, string) {
	now := time.Now()
	startOfYear := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
	endOfYear := time.Date(now.Year(), 12, 31, 23, 59, 59, 999999999, now.Location())

	completedPaths, err := u.readingRepository.CompletedBetweenDates(userID, &startOfYear, &endOfYear)
	if err != nil {
		log.Printf("error getting completed readings for user %d: %s\n", userID, err)
		return 0, ""
	}

	return u.calculateReadingStats(completedPaths, userID, wordsPerMinute)
}

func (u *Controller) calculateLifetimeStats(userID int, wordsPerMinute float64) (int, string) {
	// Get all completed documents (no date filtering)
	completedPaths, err := u.readingRepository.CompletedBetweenDates(userID, nil, nil)
	if err != nil {
		log.Printf("error getting lifetime completed readings for user %d: %s\n", userID, err)
		return 0, ""
	}

	return u.calculateReadingStats(completedPaths, userID, wordsPerMinute)
}

func (u *Controller) calculateReadingStats(completedPaths []string, userID int, wordsPerMinute float64) (int, string) {
	if len(completedPaths) == 0 {
		return 0, ""
	}

	totalWords, err := u.indexer.TotalWordCount(completedPaths)
	if err != nil {
		log.Printf("error getting total word count for user %d: %s\n", userID, err)
		return len(completedPaths), ""
	}

	// Calculate reading time and format it
	if readingTime, err := time.ParseDuration(fmt.Sprintf("%fm", totalWords/wordsPerMinute)); err == nil {
		return len(completedPaths), metadata.FmtDuration(readingTime)
	}

	return len(completedPaths), ""
}
