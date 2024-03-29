package webserver_test

import (
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v3/internal/webserver/infrastructure"
	"github.com/svera/coreander/v3/internal/webserver/model"
)

func TestSearch(t *testing.T) {
	db := infrastructure.Connect("file::memory:", 250)
	smtpMock := &SMTPMock{}
	appFS := loadDirInMemoryFs("fixtures/library")

	app := bootstrapApp(db, smtpMock, appFS)

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
	app := bootstrapApp(db, smtpMock, afero.NewOsFs())

	var cases = []struct {
		name               string
		email              string
		slug               string
		expectedHTTPStatus int
	}{
		{"Send no document slug", "admin@example.com", "", http.StatusBadRequest},
		{"Send no email address", "", "empty", http.StatusBadRequest},
		{"Send non existing document slug", "admin@example.com", "wrong", http.StatusBadRequest},
		{"Send document slug and email address", "admin@example.com", "john-doe-test-epub", http.StatusOK},
	}

	for _, tcase := range cases {
		t.Run(tcase.name, func(t *testing.T) {
			var (
				response *http.Response
				err      error
			)

			data := url.Values{
				"email": {tcase.email},
				"slug":  {tcase.slug},
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

			if response.StatusCode != tcase.expectedHTTPStatus {
				t.Errorf("Expected status %d, received %d", tcase.expectedHTTPStatus, response.StatusCode)
			}
		})
	}
}

func TestRemoveDocument(t *testing.T) {
	db := infrastructure.Connect("file::memory:", 250)
	smtpMock := &SMTPMock{}
	appFS := loadDirInMemoryFs("fixtures/library")
	app := bootstrapApp(db, smtpMock, appFS)

	assertSearchResults(app, t, "john+doe", 4)

	user := &model.User{
		Uuid:           uuid.NewString(),
		Name:           "regular",
		Email:          "regular@example.com",
		Password:       model.Hash("regular"),
		Role:           model.RoleRegular,
		WordsPerMinute: 50,
	}
	result := db.Create(&user)
	if result.Error != nil {
		log.Fatal("Couldn't create regular user")
	}

	var cases = []struct {
		name               string
		email              string
		password           string
		file               string
		slug               string
		expectedHTTPStatus int
	}{
		{"Remove no document slug", "admin@example.com", "admin", "", "", http.StatusBadRequest},
		{"Remove non existing document slug", "admin@example.com", "admin", "wrong.epub", "", http.StatusBadRequest},
		{"Remove document with a regular user", "regular@example.com", "regular", "metadata.epub", "john-doe-test-epub", http.StatusForbidden},
		{"Remove document with an admin user", "admin@example.com", "admin", "metadata.epub", "john-doe-test-epub", http.StatusOK},
	}

	for _, tcase := range cases {
		t.Run(tcase.name, func(t *testing.T) {
			var (
				response *http.Response
				err      error
			)

			data := url.Values{
				"slug": {tcase.slug},
			}

			cookie, err := login(app, tcase.email, tcase.password)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}

			req, err := http.NewRequest(http.MethodDelete, "/document", strings.NewReader(data.Encode()))
			if err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}
			req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
			req.AddCookie(cookie)

			response, err = app.Test(req)

			if err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}

			if tcase.expectedHTTPStatus == http.StatusOK {
				if _, err := appFS.Stat(tcase.file); !os.IsNotExist(err) {
					t.Errorf("Expected 'file not exist' error when trying to access a file that should have been removed")
				}

				assertSearchResults(app, t, "john+doe", 3)

			}

			if response.StatusCode != tcase.expectedHTTPStatus {
				t.Errorf("Expected status %d, received %d", tcase.expectedHTTPStatus, response.StatusCode)
			}
		})
	}
}

func TestDocumentAndRead(t *testing.T) {
	db := infrastructure.Connect("file::memory:", 250)
	smtpMock := &SMTPMock{}
	app := bootstrapApp(db, smtpMock, afero.NewOsFs())

	var cases = []struct {
		url            string
		expectedStatus int
	}{
		{"/en/read/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha", http.StatusOK},
		{"/en/read/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha-2", http.StatusOK},
		{"/en/document/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha", http.StatusOK},
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

func loadFilesInMemoryFs(files []string) afero.Fs {
	var (
		contents map[string][]byte
	)

	appFS := afero.NewMemMapFs()

	for _, fileName := range files {
		file, err := os.Open(fileName)
		if err != nil {
			log.Fatalf("Couldn't open %s", fileName)
		}
		_, err = file.Read(contents[fileName])
		if err != nil {
			log.Fatalf("Couldn't read contents of %s", fileName)
		}
		afero.WriteFile(appFS, fileName, contents[fileName], 0644)
	}
	return appFS
}

func assertSearchResults(app *fiber.App, t *testing.T, search string, expectedResults int) {
	req, err := http.NewRequest(http.MethodGet, "/en?search="+search, nil)
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
