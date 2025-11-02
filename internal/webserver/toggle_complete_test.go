package webserver_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/webserver"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"github.com/svera/coreander/v4/internal/webserver/model"
	"gorm.io/gorm"
)

func TestToggleComplete(t *testing.T) {
	var (
		db            *gorm.DB
		app           *fiber.App
		adminCookie   *http.Cookie
		regularCookie *http.Cookie
	)

	reset := func() {
		t.Helper()

		var err error
		db = infrastructure.Connect(":memory:", 250)

		appFs := loadFilesInMemoryFs([]string{
			"fixtures/library/metadata.epub",
			"fixtures/library/quijote.epub",
		})

		webserverConfig := webserver.Config{
			SessionTimeout: 24 * time.Hour,
			LibraryPath:    "fixtures/library",
			WordsPerMinute: 250,
		}

		app = bootstrapApp(db, &infrastructure.NoEmail{}, appFs, webserverConfig)

		adminCookie, err = login(app, "admin@example.com", "admin", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		// Create a regular user for testing
		regularUserData := url.Values{
			"name":             {"Regular"},
			"username":         {"regular"},
			"email":            {"regular@example.com"},
			"password":         {"regular"},
			"confirm-password": {"regular"},
			"role":             {fmt.Sprint(model.RoleRegular)},
			"words-per-minute": {"250"},
		}

		response, err := postRequest(regularUserData, adminCookie, app, "/users", t)
		if response == nil || err != nil {
			t.Fatalf("Unexpected error creating regular user: %v", err)
		}

		regularCookie, err = login(app, "regular@example.com", "regular", t)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
	}

	t.Run("Try to mark document as complete without authentication", func(t *testing.T) {
		reset()

		req, _ := http.NewRequest(http.MethodPost, "/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha/complete", nil)
		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		if response.StatusCode != fiber.StatusForbidden && response.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("Expected 401 or 403 status, got %d", response.StatusCode)
		}
	})

	t.Run("Mark document as complete successfully", func(t *testing.T) {
		reset()

		req, _ := http.NewRequest(http.MethodPost, "/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha/complete", nil)
		req.AddCookie(regularCookie)
		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		if response.StatusCode != fiber.StatusNoContent {
			t.Errorf("Expected 204 status, got %d", response.StatusCode)
		}

		// Verify document is marked as complete in database
		var reading model.Reading
		err = db.Where("user_id = ? AND path = ?", 2, "quijote.epub").First(&reading).Error
		if err != nil {
			t.Fatalf("Expected reading record to exist: %v", err)
		}

		if !reading.Completed {
			t.Error("Expected Completed to be true")
		}

		if reading.CompletedAt == nil {
			t.Error("Expected CompletedAt to be set")
		}
	})

	t.Run("Toggle document from complete to incomplete", func(t *testing.T) {
		reset()

		// First mark as complete
		req, _ := http.NewRequest(http.MethodPost, "/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha/complete", nil)
		req.AddCookie(regularCookie)
		_, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		// Verify it's complete
		var reading model.Reading
		err = db.Where("user_id = ? AND path = ?", 2, "quijote.epub").First(&reading).Error
		if err != nil || !reading.Completed || reading.CompletedAt == nil {
			t.Fatal("Document should be marked as complete")
		}

		// Toggle to incomplete
		req, _ = http.NewRequest(http.MethodPost, "/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha/complete", nil)
		req.AddCookie(regularCookie)
		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		if response.StatusCode != fiber.StatusNoContent {
			t.Errorf("Expected 204 status, got %d", response.StatusCode)
		}

		// Verify it's now incomplete
		var readingAfterToggle model.Reading
		err = db.Where("user_id = ? AND path = ?", 2, "quijote.epub").First(&readingAfterToggle).Error
		if err != nil {
			t.Fatalf("Expected reading record to exist: %v", err)
		}

		if readingAfterToggle.Completed {
			t.Error("Expected Completed to be false")
		}

		if readingAfterToggle.CompletedAt != nil {
			t.Error("Expected CompletedAt to be nil")
		}
	})

	t.Run("Complete button shows correct state in UI", func(t *testing.T) {
		reset()

		// Get document detail page - should show incomplete
		req, _ := http.NewRequest(http.MethodGet, "/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha", nil)
		req.AddCookie(regularCookie)
		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		doc, err := goquery.NewDocumentFromReader(response.Body)
		if err != nil {
			t.Fatal(err)
		}

		// Check for incomplete icon (bi-circle)
		if doc.Find("i.bi-circle").Length() == 0 {
			t.Error("Expected incomplete icon (bi-circle) to be present")
		}

		// Mark as complete
		req, _ = http.NewRequest(http.MethodPost, "/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha/complete", nil)
		req.AddCookie(regularCookie)
		_, err = app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error marking complete: %v", err.Error())
		}

		// Get document detail page again - should show complete
		req, _ = http.NewRequest(http.MethodGet, "/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha", nil)
		req.AddCookie(regularCookie)
		response, err = app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		doc, err = goquery.NewDocumentFromReader(response.Body)
		if err != nil {
			t.Fatal(err)
		}

		// Check for complete icon (bi-check-circle-fill)
		if doc.Find("i.bi-check-circle-fill").Length() == 0 {
			t.Error("Expected complete icon (bi-check-circle-fill) to be present")
		}

		// Check for success class on button
		if doc.Find("button.btn-success").Length() == 0 {
			t.Error("Expected button to have btn-success class when complete")
		}
	})

	t.Run("Different users have independent completion status", func(t *testing.T) {
		reset()

		// Regular user marks as complete
		req, _ := http.NewRequest(http.MethodPost, "/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha/complete", nil)
		req.AddCookie(regularCookie)
		_, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		// Check admin user sees it as incomplete
		var reading model.Reading
		err = db.Where("user_id = ? AND path = ?", 1, "quijote.epub").First(&reading).Error
		if err != gorm.ErrRecordNotFound {
			if reading.Completed || reading.CompletedAt != nil {
				t.Error("Admin user should not see document as complete")
			}
		}
	})

	t.Run("Try to mark non-existent document as complete", func(t *testing.T) {
		reset()

		req, _ := http.NewRequest(http.MethodPost, "/documents/non-existent-doc/complete", nil)
		req.AddCookie(regularCookie)
		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		if response.StatusCode != fiber.StatusNotFound {
			t.Errorf("Expected 404 status for non-existent document, got %d", response.StatusCode)
		}
	})

	t.Run("Update completion date while keeping document marked as complete", func(t *testing.T) {
		reset()

		// First mark as complete
		req, _ := http.NewRequest(http.MethodPost, "/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha/complete", nil)
		req.AddCookie(regularCookie)
		_, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		// Verify it's complete with a date
		var reading model.Reading
		err = db.Where("user_id = ? AND path = ?", 2, "quijote.epub").First(&reading).Error
		if err != nil {
			t.Fatalf("Expected reading record to exist: %v", err)
		}

		if !reading.Completed {
			t.Fatal("Document should be marked as complete")
		}

		if reading.CompletedAt == nil {
			t.Fatal("CompletedAt should be set")
		}

		originalDate := *reading.CompletedAt

		// Update the completion date to a different date
		newDate := "2024-01-15"
		reqBody := fmt.Sprintf(`{"completed_at":"%s"}`, newDate)
		req, _ = http.NewRequest(http.MethodPut, "/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha/complete-date", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(regularCookie)
		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		if response.StatusCode != fiber.StatusNoContent {
			t.Errorf("Expected 204 status, got %d", response.StatusCode)
		}

		// Verify it's still complete but with the new date
		var readingAfterUpdate model.Reading
		err = db.Where("user_id = ? AND path = ?", 2, "quijote.epub").First(&readingAfterUpdate).Error
		if err != nil {
			t.Fatalf("Expected reading record to exist: %v", err)
		}

		if !readingAfterUpdate.Completed {
			t.Error("Document should still be marked as complete")
		}

		if readingAfterUpdate.CompletedAt == nil {
			t.Error("CompletedAt should still be set")
		}

		// Verify the date was actually updated
		expectedDate, _ := time.Parse("2006-01-02", newDate)
		if readingAfterUpdate.CompletedAt.Format("2006-01-02") != expectedDate.Format("2006-01-02") {
			t.Errorf("Expected completion date to be %s, got %s", newDate, readingAfterUpdate.CompletedAt.Format("2006-01-02"))
		}

		// Verify it's different from original date
		if readingAfterUpdate.CompletedAt.Format("2006-01-02") == originalDate.Format("2006-01-02") {
			t.Error("Completion date should have been updated")
		}
	})

	t.Run("Try to update completion date without authentication", func(t *testing.T) {
		reset()

		reqBody := `{"completed_at":"2024-01-15"}`
		req, _ := http.NewRequest(http.MethodPut, "/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha/complete-date", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		if response.StatusCode != fiber.StatusForbidden && response.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("Expected 401 or 403 status, got %d", response.StatusCode)
		}
	})

	t.Run("Try to update completion date with invalid date format", func(t *testing.T) {
		reset()

		// First mark as complete
		req, _ := http.NewRequest(http.MethodPost, "/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha/complete", nil)
		req.AddCookie(regularCookie)
		_, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		// Try to update with invalid date format
		reqBody := `{"completed_at":"invalid-date"}`
		req, _ = http.NewRequest(http.MethodPut, "/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha/complete-date", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(regularCookie)
		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		if response.StatusCode != fiber.StatusBadRequest {
			t.Errorf("Expected 400 status for invalid date, got %d", response.StatusCode)
		}
	})
}
