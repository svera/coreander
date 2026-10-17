package webserver_test

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"

	gowikidata "github.com/Navid2zp/go-wikidata"
	"github.com/PuerkitoBio/goquery"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/datasource/wikidata"
	"github.com/svera/coreander/v5/internal/webserver"
	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
)

func TestClearAuthorRemovesImage(t *testing.T) {
	mockDataSourceServer := wikidata.NewMockServer(t, "fixtures/datasource/wikidata")
	gowikidata.WikidataDomain = mockDataSourceServer.URL
	defer mockDataSourceServer.Close()

	defaultImage, err := os.ReadFile("embedded/images/male.webp")
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name   string
		style  string
		cached bool
	}{
		{"summary with cached image", "", true},
		{"document detail with cached image", "?style=clear", true},
		{"summary without cached image", "", false},
		{"document detail without cached image", "?style=clear", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := infrastructure.Connect(":memory:", 250)
			appFS := loadDirInMemoryFs("testdata/library")
			cfg := defaultTestConfig()
			cfg.CacheDir = "/cache"
			app := bootstrapApp(db, &infrastructure.NoEmail{}, appFS, cfg)

			cookie, err := login(app, "admin@example.com", "admin", t)
			if err != nil {
				t.Fatal(err)
			}
			response, err := getRequest(cookie, app, "/authors/john-doe/summary", t)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			mustReturnStatus(response, http.StatusOK, t)

			imagePath := cfg.CacheDir + "/authors/john-doe.webp"
			if tc.cached {
				if err := appFS.MkdirAll(cfg.CacheDir+"/authors", 0755); err != nil {
					t.Fatal(err)
				}
				if err := afero.WriteFile(appFS, imagePath, []byte("old author image"), 0644); err != nil {
					t.Fatal(err)
				}
			}

			response, err = putRequest(url.Values{"sourceID": {""}}, cookie, app, "/authors/john-doe"+tc.style, t)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			mustReturnStatus(response, http.StatusOK, t)
			if _, err := appFS.Stat(imagePath); !os.IsNotExist(err) {
				t.Fatalf("expected cached image to be absent, got %v", err)
			}

			doc, err := goquery.NewDocumentFromReader(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			imageURL, _ := doc.Find("img").Attr("src")
			if imageURL != "/authors/john-doe_.webp" {
				t.Fatalf("expected image URL with cleared ID and no cached version, got %q", imageURL)
			}
			if doc.Find("dt:contains('Date of birth')").Length() != 0 {
				t.Fatal("expected author information to be cleared")
			}

			response, err = getRequest(cookie, app, imageURL, t)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			mustReturnStatus(response, http.StatusOK, t)
			imageData, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(imageData, defaultImage) {
				t.Fatal("expected default author image after clearing Wikidata ID")
			}

			response, err = getRequest(cookie, app, "/authors/john-doe/summary"+tc.style, t)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			mustReturnStatus(response, http.StatusOK, t)
			doc, err = goquery.NewDocumentFromReader(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if imageURL, _ := doc.Find("img").Attr("src"); imageURL != "/authors/john-doe_.webp" {
				t.Fatalf("expected cleared image URL to persist, got %q", imageURL)
			}
		})
	}
}

func TestAuthorSummary(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	smtpMock := &infrastructure.SMTPMock{}
	appFS := loadDirInMemoryFs("testdata/library")
	mockDataSourceServer := wikidata.NewMockServer(t, "fixtures/datasource/wikidata")

	gowikidata.WikidataDomain = mockDataSourceServer.URL

	defer mockDataSourceServer.Close()

	app := bootstrapApp(db, smtpMock, appFS, webserver.Config{})

	var cases = []struct {
		name                string
		url                 string
		expectedStatus      int
		expectedDateOfBirth int
	}{
		{"Search for authors", "/authors/john-doe/summary", http.StatusOK, 1},
		{"Search for authors not indexed", "/authors/not-found/summary", http.StatusNotFound, 0},
		{"Search for authors indexed but not in Wikidata", "/authors/miguel-de-cervantes-y-saavedra/summary", http.StatusNotFound, 0},
	}

	for _, tcase := range cases {
		t.Run(tcase.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, tcase.url, nil)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}
			req.Header.Set("Accept-Language", "en")
			response, err := app.Test(req)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err.Error())
			}
			if expectedStatus := tcase.expectedStatus; response.StatusCode != expectedStatus {
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
