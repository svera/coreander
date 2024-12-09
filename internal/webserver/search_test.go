package webserver_test

import (
	"net/http"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/webserver"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
)

func TestSearch(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	smtpMock := &infrastructure.SMTPMock{}
	appFS := loadDirInMemoryFs("fixtures/library")

	app := bootstrapApp(db, smtpMock, appFS, webserver.Config{})

	var cases = []struct {
		name            string
		url             string
		expectedResults int
	}{
		{"Search for documents with no metadata", "/documents?search=empty", 2},
		{"Search for documents with metadata", "/documents?search=john+doe", 4},
		{"Search for authors", "/authors/john-doe", 4},
	}

	for _, tcase := range cases {
		t.Run(tcase.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, tcase.url, nil)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}
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

			if actualResults := doc.Find(".list-group-item").Length(); actualResults != tcase.expectedResults {
				t.Errorf("Expected %d results, got %d", tcase.expectedResults, actualResults)
			}
		})
	}
}

func assertSearchResults(app *fiber.App, t *testing.T, search string, expectedResults int) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, "/documents?search="+search, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
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

	if actualResults := doc.Find(".list-group-item").Length(); actualResults != expectedResults {
		t.Errorf("Expected %d results, got %d", expectedResults, actualResults)
	}
}
