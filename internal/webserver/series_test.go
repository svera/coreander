package webserver_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/svera/coreander/v5/internal/metadata"
	"github.com/svera/coreander/v5/internal/precisiondate"
	"github.com/svera/coreander/v5/internal/webserver"
	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
)

func TestSeries(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	smtpMock := &infrastructure.SMTPMock{}
	appFS := loadDirInMemoryFs("testdata/library")

	app := bootstrapApp(db, smtpMock, appFS, webserver.Config{})

	var cases = []struct {
		name           string
		url            string
		expectedStatus int
	}{
		{"Access to existing series page", "/series/the-lord-of-the-rings", http.StatusOK},
		{"Access to non existing series page", "/series/non-existing-series", http.StatusNotFound},
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
			if expectedStatus := tcase.expectedStatus; response.StatusCode != expectedStatus {
				t.Errorf("Expected status %d, received %d", expectedStatus, response.StatusCode)
			}
			response.Body.Close()
		})
	}
}

func TestSeriesFilters(t *testing.T) {
	catalog := libraryCatalog()
	first := catalog[testLibraryDir+"/metadata.epub"]
	first.Title = "First volume"
	first.SeriesIndex = 1
	first.Words = 15000
	catalog[testLibraryDir+"/metadata.epub"] = first
	second := first
	second.Title = "Second volume"
	second.SeriesIndex = 2
	second.Language = "es"
	second.Subjects = []string{"Adventure"}
	second.Publication = precisiondate.NewPrecisionDate("2021-01-01T00:00:00Z", precisiondate.PrecisionDay)
	second.Words = 30000
	catalog[testLibraryDir+"/nested/other.epub"] = second
	third := first
	third.Title = "Third volume"
	third.SeriesIndex = 3
	third.Format = "PDF"
	third.Pages = 100
	third.Words = 0
	catalog[testLibraryDir+"/metadata.pdf"] = third
	reader := catalogReader{byPath: catalog}
	app := bootstrapApp(infrastructure.Connect(":memory:", 250), &infrastructure.NoEmail{},
		loadDirInMemoryFs(testLibraryDir), webserver.Config{},
		map[string]metadata.Reader{".epub": reader, ".pdf": reader})

	cases := []struct {
		name   string
		query  string
		titles []string
	}{
		{"default series order", "", []string{"First volume", "Second volume", "Third volume"}},
		{"descending series order", "?sort-by=number-desc", []string{"Third volume", "Second volume", "First volume"}},
		{"language", "?language=es", []string{"Second volume"}},
		{"subjects", "?subjects=fiction", []string{"First volume", "Third volume"}},
		{"publishing date", "?pub-date-from=2021-01-01", []string{"Second volume"}},
		{"inclusive date boundary", "?pub-date-to=2020-01-01", []string{"First volume", "Third volume"}},
		{"reading time", "?est-read-time-from=1&est-read-time-to=1", []string{"First volume"}},
		{"pages", "?pages-from=100&pages-to=100", []string{"Third volume"}},
		{"combined filters and sort", "?language=en&subjects=fiction&sort-by=number-desc", []string{"Third volume", "First volume"}},
		{"no matching documents", "?subjects=non-existing-subject", nil},
		{"out of range page", "?page=99", nil},
	}
	for _, tc := range cases {
		for _, htmx := range []bool{false, true} {
			name := tc.name
			if htmx {
				name += " HTMX"
			}
			t.Run(name, func(t *testing.T) {
				req, err := http.NewRequest(http.MethodGet, "/series/the-lord-of-the-rings"+tc.query, nil)
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Accept-Language", "en")
				if htmx {
					req.Header.Set("HX-Request", "true")
				}
				response, err := app.Test(req)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				if response.StatusCode != http.StatusOK {
					t.Fatalf("Expected status 200, got %d", response.StatusCode)
				}
				doc, err := goquery.NewDocumentFromReader(response.Body)
				if err != nil {
					t.Fatal(err)
				}
				listID := "#list"
				if htmx {
					listID = "#list-fragment-body"
					if value, _ := doc.Find("#series-list-header").Attr("hx-swap-oob"); value != "true" {
						t.Error("Expected out-of-band collection header")
					}
					if doc.Find("#list-header, #search-offcanvas-tabs").Length() != 0 {
						t.Error("Collection update must not replace the global search header")
					}
				} else {
					if action, _ := doc.Find("#search-filters-form").Attr("action"); action != "/series/the-lord-of-the-rings" {
						t.Errorf("Unexpected filters form action: %s", action)
					}
					if doc.Find("#series-search-filters-offcanvas #document-search-filters-sidebar").Length() != 1 {
						t.Error("Expected responsive document filters panel")
					}
					if target, _ := doc.Find("#series-list-header button").Attr("data-bs-target"); target != "#series-search-filters-offcanvas" {
						t.Errorf("Unexpected mobile filters target: %s", target)
					}
					if include, _ := doc.Find("#list").Attr("hx-include"); include != "#search-filters-form, #sort-by" {
						t.Errorf("Unexpected list filter inclusion: %s", include)
					}
					if tc.query == "?language=es" {
						if value, _ := doc.Find("#sidebar-language option[selected]").Attr("value"); value != "es" {
							t.Error("Expected selected language to survive reload")
						}
					}
				}
				var titles []string
				doc.Find(listID + " h2 a").Each(func(_ int, s *goquery.Selection) {
					titles = append(titles, s.Text())
				})
				if !slices.Equal(titles, tc.titles) {
					t.Errorf("Expected titles %v, got %v", tc.titles, titles)
				}
				if len(tc.titles) == 0 && tc.query != "?page=99" {
					if strings.TrimSpace(doc.Find(listID).Text()) != "No results found" {
						t.Error("Expected empty-results message")
					}
				}
			})
		}
	}
	for _, url := range []string{
		"/series/the-lord-of-the-rings?pub-date-from=invalid",
		"/series/the-lord-of-the-rings?pub-date-to=invalid",
	} {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected invalid date to return 400, got %d", response.StatusCode)
		}
	}
}
