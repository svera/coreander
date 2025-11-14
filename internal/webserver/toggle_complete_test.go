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

		if reading.CompletedOn == nil {
			t.Error("Expected CompletedOn to be set (document should be marked as complete)")
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
		if err != nil || reading.CompletedOn == nil {
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

		if readingAfterToggle.CompletedOn != nil {
			t.Error("Expected CompletedOn to be nil (document should be marked as incomplete)")
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

		// Check for unchecked checkbox
		checkbox := doc.Find("input[type='checkbox'][id^='complete-checkbox-']")
		if checkbox.Length() == 0 {
			t.Error("Expected completion checkbox to be present")
		}

		if _, exists := checkbox.Attr("checked"); exists {
			t.Error("Expected checkbox to be unchecked when document is incomplete")
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

		// Check for checked checkbox
		completedCheckbox := doc.Find("input[type='checkbox'][id^='complete-checkbox-']")
		if completedCheckbox.Length() == 0 {
			t.Error("Expected completion checkbox to be present")
		}

		if _, exists := completedCheckbox.Attr("checked"); !exists {
			t.Error("Expected checkbox to be checked when document is complete")
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
			if reading.CompletedOn != nil {
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

		if reading.CompletedOn == nil {
			t.Fatal("CompletedOn should be set (document should be marked as complete)")
		}

		originalDate := *reading.CompletedOn

		// Update the completion date to a different date
		newDate := "2024-01-15"
		reqBody := fmt.Sprintf(`{"completed_on":"%s"}`, newDate)
		req, _ = http.NewRequest(http.MethodPut, "/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha/complete", strings.NewReader(reqBody))
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

		if readingAfterUpdate.CompletedOn == nil {
			t.Error("CompletedOn should still be set (document should be marked as complete)")
		}

		// Verify the date was actually updated
		expectedDate, _ := time.Parse("2006-01-02", newDate)
		if readingAfterUpdate.CompletedOn.Format("2006-01-02") != expectedDate.Format("2006-01-02") {
			t.Errorf("Expected completion date to be %s, got %s", newDate, readingAfterUpdate.CompletedOn.Format("2006-01-02"))
		}

		// Verify it's different from original date
		if readingAfterUpdate.CompletedOn.Format("2006-01-02") == originalDate.Format("2006-01-02") {
			t.Error("Completion date should have been updated")
		}
	})

	t.Run("Try to update completion date without authentication", func(t *testing.T) {
		reset()

		reqBody := `{"completed_on":"2024-01-15"}`
		req, _ := http.NewRequest(http.MethodPut, "/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha/complete", strings.NewReader(reqBody))
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
		reqBody := `{"completed_on":"invalid-date"}`
		req, _ = http.NewRequest(http.MethodPut, "/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha/complete", strings.NewReader(reqBody))
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

	t.Run("Updating completion status or date does not change updated_at timestamp", func(t *testing.T) {
		reset()

		// First, update the reading position to establish an updated_at timestamp
		positionBody := `{"position":"test-position"}`
		req, _ := http.NewRequest(http.MethodPut, "/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha/position", strings.NewReader(positionBody))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(regularCookie)
		_, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		// Get the initial updated_at timestamp
		var initialReading model.Reading
		err = db.Where("user_id = ? AND path = ?", 2, "quijote.epub").First(&initialReading).Error
		if err != nil {
			t.Fatalf("Expected reading record to exist: %v", err)
		}
		initialUpdatedAt := initialReading.UpdatedAt

		// Wait a bit to ensure time difference would be detectable
		time.Sleep(10 * time.Millisecond)

		// Mark document as complete
		req, _ = http.NewRequest(http.MethodPost, "/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha/complete", nil)
		req.AddCookie(regularCookie)
		_, err = app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error marking complete: %v", err.Error())
		}

		// Check that updated_at has NOT changed
		var afterCompleteReading model.Reading
		err = db.Where("user_id = ? AND path = ?", 2, "quijote.epub").First(&afterCompleteReading).Error
		if err != nil {
			t.Fatalf("Expected reading record to exist: %v", err)
		}

		if !afterCompleteReading.UpdatedAt.Equal(initialUpdatedAt) {
			t.Errorf("UpdatedAt should not change when marking document as complete. Before: %v, After: %v",
				initialUpdatedAt, afterCompleteReading.UpdatedAt)
		}

		// Wait again
		time.Sleep(10 * time.Millisecond)

		// Change the completion date
		newDate := "2024-01-15"
		reqBody := fmt.Sprintf(`{"completed_on":"%s"}`, newDate)
		req, _ = http.NewRequest(http.MethodPut, "/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha/complete", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(regularCookie)
		_, err = app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error updating completion date: %v", err.Error())
		}

		// Check that updated_at STILL has not changed
		var afterDateUpdateReading model.Reading
		err = db.Where("user_id = ? AND path = ?", 2, "quijote.epub").First(&afterDateUpdateReading).Error
		if err != nil {
			t.Fatalf("Expected reading record to exist: %v", err)
		}

		if !afterDateUpdateReading.UpdatedAt.Equal(initialUpdatedAt) {
			t.Errorf("UpdatedAt should not change when updating completion date. Before: %v, After: %v",
				initialUpdatedAt, afterDateUpdateReading.UpdatedAt)
		}

		// Wait again
		time.Sleep(10 * time.Millisecond)

		// Toggle back to incomplete
		req, _ = http.NewRequest(http.MethodPost, "/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha/complete", nil)
		req.AddCookie(regularCookie)
		_, err = app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error toggling to incomplete: %v", err.Error())
		}

		// Check that updated_at STILL has not changed
		var afterIncompleteReading model.Reading
		err = db.Where("user_id = ? AND path = ?", 2, "quijote.epub").First(&afterIncompleteReading).Error
		if err != nil {
			t.Fatalf("Expected reading record to exist: %v", err)
		}

		if !afterIncompleteReading.UpdatedAt.Equal(initialUpdatedAt) {
			t.Errorf("UpdatedAt should not change when marking document as incomplete. Before: %v, After: %v",
				initialUpdatedAt, afterIncompleteReading.UpdatedAt)
		}
	})

	t.Run("Null updated_at remains null after marking document as complete", func(t *testing.T) {
		reset()

		// Create a reading record with null updated_at using Touch
		readingRepo := &model.ReadingRepository{DB: db}
		err := readingRepo.Touch(2, "quijote.epub")
		if err != nil {
			t.Fatalf("Unexpected error creating reading record: %v", err)
		}

		// Verify updated_at is null in the database
		var initialReading model.Reading
		err = db.Raw("SELECT user_id, path, position, created_at, updated_at, completed_on FROM readings WHERE user_id = ? AND path = ?", 2, "quijote.epub").Scan(&initialReading).Error
		if err != nil {
			t.Fatalf("Expected reading record to exist: %v", err)
		}

		// Check if updated_at is the zero value (which indicates NULL in the database)
		if !initialReading.UpdatedAt.IsZero() {
			t.Error("Expected UpdatedAt to be zero (NULL) after Touch")
		}

		// Mark document as complete
		req, _ := http.NewRequest(http.MethodPost, "/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha/complete", nil)
		req.AddCookie(regularCookie)
		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error marking complete: %v", err.Error())
		}

		if response.StatusCode != fiber.StatusNoContent {
			t.Errorf("Expected 204 status, got %d", response.StatusCode)
		}

		// Verify updated_at is still null in the database
		var afterCompleteReading model.Reading
		err = db.Raw("SELECT user_id, path, position, created_at, updated_at, completed_on FROM readings WHERE user_id = ? AND path = ?", 2, "quijote.epub").Scan(&afterCompleteReading).Error
		if err != nil {
			t.Fatalf("Expected reading record to exist: %v", err)
		}

		// Check that updated_at is still zero (NULL)
		if !afterCompleteReading.UpdatedAt.IsZero() {
			t.Errorf("Expected UpdatedAt to remain NULL after marking as complete, but got: %v", afterCompleteReading.UpdatedAt)
		}

		// Verify the document was actually marked as complete
		if afterCompleteReading.CompletedOn == nil {
			t.Error("Expected CompletedOn to be set (document should be marked as complete)")
		}
	})
}
