package webserver_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/svera/coreander/internal/infrastructure"
)

func TestSearch(t *testing.T) {
	db := infrastructure.Connect("file::memory:", 250)
	smtpMock := &SMTPMock{}
	app := bootstrapApp(db, smtpMock)

	var cases = []struct {
		name            string
		url             string
		expectedResults int
	}{
		{"Search for documents with no metadata", "/en?search=empty", 2},
		{"Search for documents with metadata", "/en?search=john+doe", 4},
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

func TestSendDocument(t *testing.T) {
	db := infrastructure.Connect("file::memory:", 250)
	smtpMock := &SMTPMock{}
	app := bootstrapApp(db, smtpMock)

	var cases = []struct {
		name               string
		email              string
		file               string
		expectedHTTPStatus int
	}{
		{"Send no document filename", "admin@example.com", "", http.StatusBadRequest},
		{"Send no email address", "", "empty.epub", http.StatusBadRequest},
		{"Send document filename with relative path", "admin@example.com", "nested/../empty.epub", http.StatusBadRequest},
		{"Send non existing document filename", "admin@example.com", "wrong.epub", http.StatusBadRequest},
		{"Send document filename and email address", "admin@example.com", "metadata.epub", http.StatusOK},
	}

	for _, tcase := range cases {
		t.Run(tcase.name, func(t *testing.T) {
			var (
				response *http.Response
				err      error
			)

			data := url.Values{
				"email": {tcase.email},
				"file":  {tcase.file},
			}

			req, err := http.NewRequest(http.MethodPost, "/send", strings.NewReader(data.Encode()))
			req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
			if err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}

			if tcase.expectedHTTPStatus == http.StatusOK {
				smtpMock.wg.Add(1)
				response, err = app.Test(req)
				smtpMock.wg.Wait()
			} else {
				response, err = app.Test(req)
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}

			if expectedStatus := tcase.expectedHTTPStatus; response.StatusCode != expectedStatus {
				t.Errorf("Expected status %d, received %d", expectedStatus, response.StatusCode)
			}
		})
	}
}
