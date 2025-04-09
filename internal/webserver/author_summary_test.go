package webserver_test

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	gowikidata "github.com/Navid2zp/go-wikidata"
	"github.com/PuerkitoBio/goquery"
	"github.com/gosimple/slug"
	"github.com/svera/coreander/v4/internal/webserver"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
)

func TestAuthorSummary(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	smtpMock := &infrastructure.SMTPMock{}
	appFS := loadDirInMemoryFs("fixtures/library")
	mockDataSourceServer := newMockWikidataServer(t)

	gowikidata.WikidataDomain = mockDataSourceServer.URL

	defer mockDataSourceServer.Close()

	app := bootstrapApp(db, smtpMock, appFS, webserver.Config{})

	var cases = []struct {
		name                string
		url                 string
		expectedDateOfBirth int
	}{
		{"Search for authors", "/authors/john-doe/summary", 1},
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
			if actualResults := doc.Find("dt:contains('Date of birth')").Length(); actualResults != tcase.expectedDateOfBirth {
				t.Errorf("Expected %d results, got %d", tcase.expectedDateOfBirth, actualResults)
			}
		})
	}
}

func newMockWikidataServer(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/w/api.php") {
			queryValues := r.URL.Query()
			if queryValues.Get("action") == "wbsearchentities" {
				slug := slug.Make(queryValues.Get("search"))
				returnResponse(fmt.Sprintf("wbsearchentities-%s", slug), w)
				return
			}
			if queryValues.Get("action") == "wbgetentities" {
				id := queryValues.Get("ids")
				returnResponse(fmt.Sprintf("wbgetentities-%s", id), w)
				return
			}
		}
		t.Errorf("Expected to request '/w/api.php', got: %s", r.URL.Path)
	}))
}

func returnResponse(fixture string, w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
	filePath := fmt.Sprintf("fixtures/datasource/wikidata/%s.json", fixture)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		if strings.HasPrefix(fixture, "wbsearchentities-") {
			filePath = "fixtures/datasource/wikidata/wbsearchentities-no-results.json"
		}
		if strings.HasPrefix(fixture, "wbgetentities-") {
			filePath = "fixtures/datasource/wikidata/wbgetentities-no-such-entity.json"
		}
	}
	contents, err := os.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Couldn't read contents of %s", filePath)
	}
	if _, err = w.Write(contents); err != nil {
		log.Fatalf("Couldn't write contents of %s", filePath)
	}
}
