package webserver_test

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/webserver"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

func TestDocumentDetail(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	smtpMock := &infrastructure.SMTPMock{}
	app := bootstrapApp(db, smtpMock, afero.NewOsFs(), webserver.Config{})

	var cases = []struct {
		url            string
		expectedStatus int
	}{
		{"/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha", http.StatusOK},
		{"/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha--2", http.StatusOK},
		{"/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha--3", http.StatusOK},
		{"/documents/john-doe-non-existing-document", http.StatusNotFound},
	}

	for _, tcase := range cases {
		t.Run(tcase.url, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, tcase.url, nil)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}
			response, err := app.Test(req)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}
			if response.StatusCode != tcase.expectedStatus {
				t.Errorf("Expected status %d, received %d", tcase.expectedStatus, response.StatusCode)
			}
		})
	}
}

func TestDocumentReadAndDeleteDocumentAfterwards(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	appFS := loadFilesInMemoryFs([]string{"fixtures/library/quijote.epub"})
	smtpMock := &infrastructure.SMTPMock{}
	app := bootstrapApp(db, smtpMock, appFS, webserver.Config{})

	adminCookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}

	var cases = []struct {
		url            string
		expectedStatus int
	}{
		{"/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha", http.StatusOK},
		{"/documents/john-doe-non-existing-document", http.StatusNotFound},
	}

	for _, tcase := range cases {
		t.Run(tcase.url, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, tcase.url+"/read", nil)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}
			req.AddCookie(adminCookie)
			response, err := app.Test(req)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}
			if response.StatusCode != tcase.expectedStatus {
				t.Errorf("Expected status %d, received %d", tcase.expectedStatus, response.StatusCode)
			}

			if tcase.expectedStatus != http.StatusOK {
				return
			}

			if !isProgressSectionShownInHome(t, app, adminCookie) {
				t.Errorf("Expected to have a resume reading section in home")
			}

			if response, err = deleteRequest(url.Values{}, adminCookie, app, tcase.url, t); err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}

			if response.StatusCode != http.StatusOK {
				t.Errorf("Expected status %d, received %d", http.StatusOK, response.StatusCode)
			}

			if isProgressSectionShownInHome(t, app, adminCookie) {
				t.Errorf("Expected to not have a resume reading section in home after removing the document")
			}

			var total int64
			if db.Table("readings").Where("path = ?", "quijote.epub").Count(&total); total != 0 {
				t.Errorf("Expected no entries in DB readings table for document, got %d", total)
			}
		})
	}
}

func TestDocumentReadAndDeleteUserAfterwards(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	appFS := loadFilesInMemoryFs([]string{"fixtures/library/quijote.epub"})
	smtpMock := &infrastructure.SMTPMock{}
	app := bootstrapApp(db, smtpMock, appFS, webserver.Config{})

	adminCookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	addRegularUser(t, app, adminCookie)

	regularUser := model.User{}
	db.Where("email = ?", "regular@example.com").First(&regularUser)

	regularUserCookie, err := login(app, "regular@example.com", "regular", t)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}

	req, err := http.NewRequest(http.MethodGet, "/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha/read", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	req.AddCookie(regularUserCookie)
	response, err := app.Test(req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	if response.StatusCode != http.StatusOK {
		t.Errorf("Expected status %d, received %d", http.StatusOK, response.StatusCode)
	}

	if response, err = deleteRequest(url.Values{}, adminCookie, app, "/users/regular", t); err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}

	if response.StatusCode != http.StatusOK {
		t.Errorf("Expected status %d, received %d", http.StatusOK, response.StatusCode)
	}

	var total int64
	if db.Table("readings").Where("user_id = ?", regularUser.ID).Count(&total); total != 0 {
		t.Errorf("Expected no entries in DB readings table for user, got %d", total)
	}
}

func isProgressSectionShownInHome(t *testing.T, app *fiber.App, cookie *http.Cookie) bool {
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	req.AddCookie(cookie)
	response, err := app.Test(req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	if expectedStatus := http.StatusOK; response.StatusCode != expectedStatus {
		t.Errorf("Expected status %d, received %d", expectedStatus, response.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}

	return doc.Find("#in-progress-docs").Length() == 1
}

func TestCompletedDocumentNotShownInResumeReading(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	appFS := loadFilesInMemoryFs([]string{"fixtures/library/quijote.epub"})
	smtpMock := &infrastructure.SMTPMock{}
	app := bootstrapApp(db, smtpMock, appFS, webserver.Config{})

	adminCookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}

	// Open the document to create a reading record
	req, err := http.NewRequest(http.MethodGet, "/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha/read", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	req.AddCookie(adminCookie)
	response, err := app.Test(req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	if response.StatusCode != http.StatusOK {
		t.Errorf("Expected status %d, received %d", http.StatusOK, response.StatusCode)
	}

	// Verify the document appears in the resume reading section
	if !isProgressSectionShownInHome(t, app, adminCookie) {
		t.Errorf("Expected to have a resume reading section in home after opening document")
	}

	// Verify the document is in the carousel
	req, err = http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	req.AddCookie(adminCookie)
	response, err = app.Test(req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	doc, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the document slug appears in the resume reading carousel
	docSlug := "miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha"
	resumeReadingDocs := doc.Find("#resume-reading-docs")
	if resumeReadingDocs.Length() == 0 {
		t.Fatalf("Expected resume reading carousel to exist")
	}
	if resumeReadingDocs.Find(fmt.Sprintf(`a[href*="%s"]`, docSlug)).Length() == 0 {
		t.Errorf("Expected document to appear in resume reading carousel")
	}

	// Mark the document as complete
	req, err = http.NewRequest(http.MethodPost, "/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha/complete", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	req.AddCookie(adminCookie)
	response, err = app.Test(req)
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
	req.AddCookie(adminCookie)
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
		if resumeReadingDocsAfter.Find(fmt.Sprintf(`a[href*="%s"]`, docSlug)).Length() > 0 {
			t.Errorf("Expected document to not appear in resume reading carousel after marking as complete")
		}
	}
	// Also verify the resume reading section is not shown (since this is the only document)
	if isProgressSectionShownInHome(t, app, adminCookie) {
		t.Errorf("Expected resume reading section to not be shown after marking the only document as complete")
	}
}

func addRegularUser(t *testing.T, app *fiber.App, adminCookie *http.Cookie) {
	regularUserData := url.Values{
		"name":             {"Regular user"},
		"username":         {"regular"},
		"email":            {"regular@example.com"},
		"password":         {"regular"},
		"confirm-password": {"regular"},
		"role":             {fmt.Sprint(model.RoleRegular)},
		"words-per-minute": {"250"},
	}

	if _, err := postRequest(regularUserData, adminCookie, app, "/users", t); err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
}
