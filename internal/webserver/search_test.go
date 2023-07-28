package webserver_test

import (
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v3/internal/infrastructure"
	"github.com/svera/coreander/v3/internal/model"
)

func TestSearch(t *testing.T) {
	db := infrastructure.Connect("file::memory:", 250)
	smtpMock := &SMTPMock{}
	app := bootstrapApp(db, smtpMock, afero.NewOsFs())

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
	fixtures := []string{"fixtures/empty.epub"}

	var (
		contents map[string][]byte
	)

	appFS := afero.NewMemMapFs()

	for _, fileName := range fixtures {
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

	app := bootstrapApp(db, smtpMock, appFS)

	req, err := http.NewRequest(http.MethodGet, "/en?search=empty", nil)
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

	if actualResults := doc.Find(".list-group-item").Length(); actualResults != 2 {
		t.Errorf("Expected %d results, got %d", 2, actualResults)
	}

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
		log.Fatal("Couldn't create default admin")
	}

	var cases = []struct {
		name               string
		email              string
		password           string
		file               string
		expectedHTTPStatus int
	}{
		{"Remove no document filename", "admin@example.com", "admin", "", http.StatusBadRequest},
		{"Remove document filename with relative path using parent path operator", "admin@example.com", "admin", "nested/../empty.epub", http.StatusBadRequest},
		{"Remove non existing document filename", "admin@example.com", "admin", "wrong.epub", http.StatusBadRequest},
		{"Remove document filename with a regular user", "regular@example.com", "regular", "empty.epub", http.StatusForbidden},
		{"Remove document filename with an admin user", "admin@example.com", "admin", "empty.epub", http.StatusOK},
	}

	for _, tcase := range cases {
		t.Run(tcase.name, func(t *testing.T) {
			var (
				response *http.Response
				err      error
			)

			data := url.Values{
				"file": {tcase.file},
			}

			cookie, err := login(app, tcase.email, tcase.password)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}

			req, err := http.NewRequest(http.MethodPost, "/delete", strings.NewReader(data.Encode()))
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
			}

			if response.StatusCode != tcase.expectedHTTPStatus {
				t.Errorf("Expected status %d, received %d", tcase.expectedHTTPStatus, response.StatusCode)
			}
		})
	}
}

func TestClashingSlugs(t *testing.T) {
	db := infrastructure.Connect("file::memory:", 250)
	smtpMock := &SMTPMock{}
	app := bootstrapApp(db, smtpMock, afero.NewOsFs())

	var cases = []struct {
		url            string
		expectedStatus int
	}{
		{"/en/read/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha", 200},
		{"/en/read/miguel-de-cervantes-y-saavedra-don-quijote-de-la-mancha-2", 200},
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
			if expectedStatus := http.StatusOK; response.StatusCode != expectedStatus {
				t.Errorf("Expected status %d, received %d", expectedStatus, response.StatusCode)
			}
		})
	}
}
