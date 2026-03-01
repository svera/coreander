package webserver_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v4/internal/webserver"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"github.com/svera/coreander/v4/internal/webserver/model"
	"gorm.io/gorm"
)

const (
	testDocSlug   = "miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha"
	testDocPath   = "quijote.epub"
	regularUserID = 2
	adminUserID   = 1
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

	// Helper functions
	markComplete := func(cookie *http.Cookie) (*http.Response, error) {
		req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/documents/%s/complete", testDocSlug), nil)
		req.AddCookie(cookie)
		return app.Test(req)
	}

	getReading := func(userID int) (model.Reading, error) {
		var reading model.Reading
		err := db.Where("user_id = ? AND path = ?", userID, testDocPath).First(&reading).Error
		return reading, err
	}

	verifyComplete := func(t *testing.T, userID int, shouldBeComplete bool) {
		t.Helper()
		reading, err := getReading(userID)
		if err != nil {
			t.Fatalf("Expected reading record to exist: %v", err)
		}
		if shouldBeComplete && reading.CompletedOn == nil {
			t.Error("Expected document to be marked as complete")
		}
		if !shouldBeComplete && reading.CompletedOn != nil {
			t.Error("Expected document to be marked as incomplete")
		}
	}

	updateCompletionDate := func(cookie *http.Cookie, date string) (*http.Response, error) {
		reqBody := fmt.Sprintf(`{"completed_on":"%s"}`, date)
		req, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("/documents/%s/complete", testDocSlug), strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(cookie)
		return app.Test(req)
	}

	t.Run("Try to mark document as complete without authentication", func(t *testing.T) {
		reset()

		req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/documents/%s/complete", testDocSlug), nil)
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

		response, err := markComplete(regularCookie)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		if response.StatusCode != fiber.StatusNoContent {
			t.Errorf("Expected 204 status, got %d", response.StatusCode)
		}

		verifyComplete(t, regularUserID, true)
	})

	t.Run("Toggle document from complete to incomplete", func(t *testing.T) {
		reset()

		// First mark as complete
		_, err := markComplete(regularCookie)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		verifyComplete(t, regularUserID, true)

		// Toggle to incomplete
		response, err := markComplete(regularCookie)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		if response.StatusCode != fiber.StatusNoContent {
			t.Errorf("Expected 204 status, got %d", response.StatusCode)
		}

		verifyComplete(t, regularUserID, false)
	})

	t.Run("Complete button shows correct state in UI", func(t *testing.T) {
		reset()

		// Get document detail page - should show incomplete
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/documents/%s", testDocSlug), nil)
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
		_, err = markComplete(regularCookie)
		if err != nil {
			t.Fatalf("Unexpected error marking complete: %v", err.Error())
		}

		// Get document detail page again - should show complete
		req, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("/documents/%s", testDocSlug), nil)
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
		_, err := markComplete(regularCookie)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		// Check admin user sees it as incomplete
		reading, err := getReading(adminUserID)
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
		_, err := markComplete(regularCookie)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		// Get original date
		reading, err := getReading(regularUserID)
		if err != nil {
			t.Fatalf("Expected reading record to exist: %v", err)
		}
		if reading.CompletedOn == nil {
			t.Fatal("CompletedOn should be set (document should be marked as complete)")
		}
		originalDate := *reading.CompletedOn

		// Update the completion date to a different date
		newDate := "2024-01-15"
		response, err := updateCompletionDate(regularCookie, newDate)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		if response.StatusCode != fiber.StatusNoContent {
			t.Errorf("Expected 204 status, got %d", response.StatusCode)
		}

		// Verify it's still complete but with the new date
		readingAfterUpdate, err := getReading(regularUserID)
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
		req, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("/documents/%s/complete", testDocSlug), strings.NewReader(reqBody))
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
		_, err := markComplete(regularCookie)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		// Try to update with invalid date format
		reqBody := `{"completed_on":"invalid-date"}`
		req, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("/documents/%s/complete", testDocSlug), strings.NewReader(reqBody))
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
		req, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("/documents/%s/position", testDocSlug), strings.NewReader(positionBody))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(regularCookie)
		_, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}

		// Get the initial updated_at timestamp
		initialReading, err := getReading(regularUserID)
		if err != nil {
			t.Fatalf("Expected reading record to exist: %v", err)
		}
		initialUpdatedAt := initialReading.UpdatedAt

		// Wait a bit to ensure time difference would be detectable
		time.Sleep(10 * time.Millisecond)

		// Mark document as complete
		_, err = markComplete(regularCookie)
		if err != nil {
			t.Fatalf("Unexpected error marking complete: %v", err.Error())
		}

		// Check that updated_at has NOT changed
		afterCompleteReading, err := getReading(regularUserID)
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
		_, err = updateCompletionDate(regularCookie, "2024-01-15")
		if err != nil {
			t.Fatalf("Unexpected error updating completion date: %v", err.Error())
		}

		// Check that updated_at STILL has not changed
		afterDateUpdateReading, err := getReading(regularUserID)
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
		_, err = markComplete(regularCookie)
		if err != nil {
			t.Fatalf("Unexpected error toggling to incomplete: %v", err.Error())
		}

		// Check that updated_at STILL has not changed
		afterIncompleteReading, err := getReading(regularUserID)
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
		err := readingRepo.Touch(regularUserID, testDocPath)
		if err != nil {
			t.Fatalf("Unexpected error creating reading record: %v", err)
		}

		// Verify updated_at is null in the database
		var initialReading model.Reading
		err = db.Raw("SELECT user_id, path, position, created_at, updated_at, completed_on FROM readings WHERE user_id = ? AND path = ?", regularUserID, testDocPath).Scan(&initialReading).Error
		if err != nil {
			t.Fatalf("Expected reading record to exist: %v", err)
		}

		// Check if updated_at is the zero value (which indicates NULL in the database)
		if !initialReading.UpdatedAt.IsZero() {
			t.Error("Expected UpdatedAt to be zero (NULL) after Touch")
		}

		// Mark document as complete
		response, err := markComplete(regularCookie)
		if err != nil {
			t.Fatalf("Unexpected error marking complete: %v", err.Error())
		}

		if response.StatusCode != fiber.StatusNoContent {
			t.Errorf("Expected 204 status, got %d", response.StatusCode)
		}

		// Verify updated_at is still null in the database
		var afterCompleteReading model.Reading
		err = db.Raw("SELECT user_id, path, position, created_at, updated_at, completed_on FROM readings WHERE user_id = ? AND path = ?", regularUserID, testDocPath).Scan(&afterCompleteReading).Error
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

	t.Run("Completed document not shown in resume reading", func(t *testing.T) {
		reset()

		// Open the document to create a reading record
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/documents/%s/read", testDocSlug), nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		req.AddCookie(regularCookie)
		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		if response.StatusCode != http.StatusOK {
			t.Errorf("Expected status %d, received %d", http.StatusOK, response.StatusCode)
		}

		// Verify the document appears in the resume reading section
		if !isProgressSectionShownInHome(t, app, regularCookie) {
			t.Errorf("Expected to have a resume reading section in home after opening document")
		}

		// Verify the document is in the carousel
		req, err = http.NewRequest(http.MethodGet, "/", nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		req.AddCookie(regularCookie)
		response, err = app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		doc, err := goquery.NewDocumentFromReader(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		// Check that the document slug appears in the resume reading carousel
		resumeReadingDocs := doc.Find("#resume-reading-docs")
		if resumeReadingDocs.Length() == 0 {
			t.Fatalf("Expected resume reading carousel to exist")
		}
		if resumeReadingDocs.Find(fmt.Sprintf(`a[href*="%s"]`, testDocSlug)).Length() == 0 {
			t.Errorf("Expected document to appear in resume reading carousel")
		}

		// Mark the document as complete
		response, err = markComplete(regularCookie)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		if response.StatusCode != http.StatusNoContent {
			t.Errorf("Expected status %d, received %d", http.StatusNoContent, response.StatusCode)
		}

		// Verify the document no longer appears in the resume reading section
		req, err = http.NewRequest(http.MethodGet, "/", nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		req.AddCookie(regularCookie)
		response, err = app.Test(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err.Error())
		}
		doc, err = goquery.NewDocumentFromReader(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		// Check that the document slug no longer appears in the resume reading carousel
		// The carousel might not exist if there are no documents, or it might exist but be empty
		resumeReadingDocsAfter := doc.Find("#resume-reading-docs")
		if resumeReadingDocsAfter.Length() > 0 {
			// If carousel exists, verify the document is not in it
			if resumeReadingDocsAfter.Find(fmt.Sprintf(`a[href*="%s"]`, testDocSlug)).Length() > 0 {
				t.Errorf("Expected document to not appear in resume reading carousel after marking as complete")
			}
		}
		// Also verify the resume reading section is not shown (since this is the only document)
		if isProgressSectionShownInHome(t, app, regularCookie) {
			t.Errorf("Expected resume reading section to not be shown after marking the only document as complete")
		}
	})
}
