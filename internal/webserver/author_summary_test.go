package webserver_test

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	gowikidata "github.com/Navid2zp/go-wikidata"
	"github.com/PuerkitoBio/goquery"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/datasource/wikidata"
	"github.com/svera/coreander/v5/internal/index"
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

func TestAuthorSummaryGoodReadsLink(t *testing.T) {
	app, idx := newGoodReadsTestApp(t, nil)
	const (
		authorPath   = "/author/show/151155.Alice_Borchardt"
		authorURL    = "https://www.goodreads.com" + authorPath
		wikipediaURL = "https://en.wikipedia.org/wiki/Alice_Borchardt"
	)
	locales := []struct {
		lang  string
		label string
	}{
		{"en", "Go to Goodreads author page"},
		{"es", "Ir a la página del autor en Goodreads"},
		{"de", "Zur Autorenseite auf Goodreads"},
		{"fr", "Voir la page de l'auteur sur Goodreads"},
		{"ru", "Перейти на страницу автора в Goodreads"},
	}
	for _, tc := range []struct {
		name      string
		goodreads bool
		wikipedia bool
		sourceID  string
	}{
		{"with-wikipedia", true, true, "Q1"},
		{"without-goodreads", false, true, "Q1"},
		{"without-wikipedia", true, false, "Q1"},
		{"without-wikidata", true, false, ""},
	} {
		author := index.Author{
			Slug: tc.name, Name: "Alice Borchardt",
			DataSourceID: tc.sourceID, RetrievedOn: time.Now().UTC(),
			WikipediaLink: make(map[string]string),
		}
		if tc.goodreads {
			author.GoodReadsAuthorURL = authorPath
		}
		if tc.wikipedia {
			for _, locale := range locales {
				author.WikipediaLink[locale.lang] = wikipediaURL
			}
		}
		if err := idx.IndexAuthor(author); err != nil {
			t.Fatal(err)
		}
		for _, style := range []struct {
			name  string
			query string
		}{
			{"summary", ""},
			{"document-detail", "?style=clear"},
		} {
			for _, locale := range locales {
				t.Run(tc.name+"/"+style.name+"/"+locale.lang, func(t *testing.T) {
					req := mustGetRequest(t, "/authors/"+tc.name+"/summary"+style.query)
					req.Header.Set("Accept-Language", locale.lang)
					response, err := app.Test(req)
					if err != nil {
						t.Fatal(err)
					}
					defer response.Body.Close()
					mustReturnStatus(response, http.StatusOK, t)
					page, err := goquery.NewDocumentFromReader(response.Body)
					if err != nil {
						t.Fatal(err)
					}
					link := page.Find(`a[href="` + authorURL + `"]`)
					if !tc.goodreads {
						if page.Find(`img[src^="/images/goodreads.svg"]`).Length() != 0 {
							t.Error("expected no Goodreads icon without an author URL")
						}
						if link.Length() != 0 {
							t.Fatal("expected no Goodreads link without an author URL")
						}
						return
					}
					if link.Length() != 1 || strings.TrimSpace(link.Text()) != locale.label {
						t.Fatalf("expected one localized Goodreads link %q, got %d links with text %q", locale.label, link.Length(), link.Text())
					}
					icon := link.PrevFiltered(`img[src^="/images/goodreads.svg"]`)
					if icon.Length() != 1 || icon.AttrOr("alt", "missing") != "" || icon.AttrOr("width", "") != "16" || icon.AttrOr("height", "") != "16" {
						t.Error("expected a decorative Goodreads SVG icon before the author link")
					}
					if tc.wikipedia {
						if href, _ := link.Parent().Prev().Find("a").Attr("href"); href != wikipediaURL {
							t.Fatalf("expected Goodreads immediately below Wikipedia, previous link is %q", href)
						}
					}
				})
			}
		}
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
