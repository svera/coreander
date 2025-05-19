package webserver_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/webserver"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
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

func TestDocumentRead(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	smtpMock := &infrastructure.SMTPMock{}
	app := bootstrapApp(db, smtpMock, afero.NewOsFs(), webserver.Config{})

	adminCookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}

	var cases = []struct {
		url            string
		expectedStatus int
	}{
		{"/documents/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha/read", http.StatusOK},
		{"/documents/john-doe-non-existing-document/read", http.StatusNotFound},
	}

	for _, tcase := range cases {
		t.Run(tcase.url, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, tcase.url, nil)
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

			if _, err = deleteRequest(url.Values{}, adminCookie, app, tcase.url, t); err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}

			if isProgressSectionShownInHome(t, app, adminCookie) {
				t.Errorf("Expected to not have a resume reading section in home after removing the document")
			}
		})
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
