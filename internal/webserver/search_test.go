package webserver_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v5/internal/webserver"
	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
)

func TestUnifiedSearch(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	smtpMock := &infrastructure.SMTPMock{}
	appFS := loadDirInMemoryFs("testdata/library")

	app := bootstrapApp(db, smtpMock, appFS, webserver.Config{})

	req, err := http.NewRequest(http.MethodGet, "/search?search=john&type=documents", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	response, err := app.Test(req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	if response.StatusCode != http.StatusOK {
		t.Errorf("Expected status %d, received %d", http.StatusOK, response.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}

	if documentResults := doc.Find("#list .list-group-item").Length(); documentResults == 0 {
		t.Error("Expected document search results for john")
	}

	req, err = http.NewRequest(http.MethodGet, "/search?search=john&type=authors", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	response, err = app.Test(req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	if response.StatusCode != http.StatusOK {
		t.Errorf("Expected status %d, received %d", http.StatusOK, response.StatusCode)
	}

	doc, err = goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}

	if authorResults := doc.Find("#list .list-group-item").Length(); authorResults == 0 {
		t.Error("Expected author search results for john")
	}
}

// TestSimilarDocumentsFullPage checks that navigating to a document's
// /similar route (the "Similar documents" > "See all" link) renders the same
// full search UI as a regular search, scoped to that document, with any
// extra filters in the query string applied - and that the Authors tab,
// which has no "similar to" concept, is hidden while in this mode. The
// scoping lives in the URL path rather than a "similar" query var, so the
// filter form's action is pointed back at that same /similar route (rather
// than the regular /search) to keep filter changes from dropping out of
// "similar to" mode.
func TestSimilarDocumentsFullPage(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	smtpMock := &infrastructure.SMTPMock{}
	appFS := loadDirInMemoryFs("testdata/library")

	app := bootstrapApp(db, smtpMock, appFS, webserver.Config{})

	req, err := http.NewRequest(http.MethodGet, "/documents/john-doe-test-epub/similar?language=en", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	response, err := app.Test(req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	if response.StatusCode != http.StatusOK {
		t.Errorf("Expected status %d, received %d", http.StatusOK, response.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}

	if doc.Find(`input[name="similar"]`).Length() != 0 {
		t.Error("Expected no hidden \"similar\" input: the scoping now lives in the URL path, not a query var")
	}

	// Only the sidebar filter form (#search-filters-form) needs to submit
	// back to the /similar route to keep filter changes in "similar to"
	// mode - the navbar offcanvas form always targets /search regardless.
	sidebarAction, _ := doc.Find(`#search-filters-form`).Attr("action")
	if sidebarAction != "/documents/john-doe-test-epub/similar" {
		t.Errorf("Expected sidebar filter form action to stay on the /similar route so filter changes keep \"similar to\" mode, got %q", sidebarAction)
	}

	if doc.Find(`[data-search-type-tab="authors"]`).Length() != 0 {
		t.Error("Expected the Authors tab to be hidden for a \"similar to\" search")
	}

	if bannerText := doc.Find(".alert-secondary").Text(); !strings.Contains(bannerText, "Test EPUB") {
		t.Errorf("Expected a banner naming the reference document, got %q", bannerText)
	}

	// The free-text search box has no effect on a "similar to" search (Search's
	// SimilarTo branch never calls composeQuery), so it should be hidden rather
	// than left present but silently ignoring whatever the user types into it.
	if doc.Find(`#sidebar-search, #searchbox-offcanvas`).Length() != 0 {
		t.Error("Expected the free-text search box to be hidden for a \"similar to\" search")
	}
}

// TestSimilarDocumentsWidgetVsFilterUpdate checks that /documents/:slug/similar
// tells apart its two htmx callers - the detail page's small preview widget
// (?widget=1, loaded on page load) and the full similar-to search page's own
// filter sidebar (which htmx-updates the same URL, without that query var,
// whenever a filter changes) - despite both being htmx requests. Regression
// test for the two being conflated by keying routing off the hx-request
// header alone, which made filter changes on the full page incorrectly hit
// the small widget instead of the filtered results fragment.
func TestSimilarDocumentsWidgetVsFilterUpdate(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	smtpMock := &infrastructure.SMTPMock{}
	appFS := loadDirInMemoryFs("testdata/library")

	app := bootstrapApp(db, smtpMock, appFS, webserver.Config{})

	widgetReq, err := http.NewRequest(http.MethodGet, "/documents/john-doe-test-epub/similar?widget=1", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	widgetReq.Header.Set("hx-request", "true")
	widgetResponse, err := app.Test(widgetReq)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	if widgetResponse.StatusCode != http.StatusOK {
		t.Errorf("Expected status %d, received %d", http.StatusOK, widgetResponse.StatusCode)
	}
	widgetDoc, err := goquery.NewDocumentFromReader(widgetResponse.Body)
	if err != nil {
		t.Fatal(err)
	}
	if widgetDoc.Find("#list-fragment-body").Length() != 0 {
		t.Error("Expected the widget response not to contain the full search results fragment")
	}

	filterReq, err := http.NewRequest(http.MethodGet, "/documents/john-doe-test-epub/similar?language=en", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	filterReq.Header.Set("hx-request", "true")
	filterResponse, err := app.Test(filterReq)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	if filterResponse.StatusCode != http.StatusOK {
		t.Errorf("Expected status %d, received %d", http.StatusOK, filterResponse.StatusCode)
	}
	filterDoc, err := goquery.NewDocumentFromReader(filterResponse.Body)
	if err != nil {
		t.Fatal(err)
	}
	if filterDoc.Find("#list-fragment-body").Length() == 0 {
		t.Error("Expected the filter-sidebar htmx update to return the full search results fragment, not the widget")
	}
}

// TestSearchKeepsSearchBoxForRegularSearch is a companion to
// TestSearchSimilarToPreservesFilterAcrossReload, checking that the free-text
// search box (hidden for "similar to" searches, since it has no effect there)
// is still shown for a regular search.
func TestSearchKeepsSearchBoxForRegularSearch(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	smtpMock := &infrastructure.SMTPMock{}
	appFS := loadDirInMemoryFs("testdata/library")

	app := bootstrapApp(db, smtpMock, appFS, webserver.Config{})

	req, err := http.NewRequest(http.MethodGet, "/search?type=documents&search=john", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	response, err := app.Test(req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	if response.StatusCode != http.StatusOK {
		t.Errorf("Expected status %d, received %d", http.StatusOK, response.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}

	if doc.Find(`#sidebar-search, #searchbox-offcanvas`).Length() == 0 {
		t.Error("Expected the free-text search box to be present for a regular search")
	}
}

func TestSearch(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	smtpMock := &infrastructure.SMTPMock{}
	appFS := loadDirInMemoryFs("testdata/library")

	app := bootstrapApp(db, smtpMock, appFS, webserver.Config{})

	var cases = []struct {
		name            string
		url             string
		expectedResults int
	}{
		{"Search for documents with no metadata", "/documents?search=empty", 2},
		{"Search for documents with metadata", "/documents?search=john+doe", 4},
		{"Search for documents with metadata using partial author name and title", "/documents?search=cervantes+quijote", 3},
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

			if actualResults := doc.Find("#list .list-group-item").Length(); actualResults != tcase.expectedResults {
				t.Errorf("Expected %d results, got %d", tcase.expectedResults, actualResults)
			}
		})
	}
}

func assertDocumentResults(app *fiber.App, t *testing.T, search string, expectedResults int) {
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

	if actualResults := doc.Find("#list .list-group-item").Length(); actualResults != expectedResults {
		t.Errorf("Expected %d results, got %d", expectedResults, actualResults)
	}
}

func assertAuthorSearchResults(app *fiber.App, t *testing.T, name string, expectedResults int) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, "/authors?name="+name, nil)
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

	if actualResults := doc.Find("#list .list-group-item").Length(); actualResults != expectedResults {
		t.Errorf("Expected %d author results, got %d", expectedResults, actualResults)
	}
}

func documentSearchFirstSlug(app *fiber.App, t *testing.T, search string) string {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, "/documents?search="+search, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	response, err := app.Test(req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, received %d", http.StatusOK, response.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}

	href, ok := doc.Find("#list .list-group-item a").First().Attr("href")
	if !ok {
		t.Fatalf("Expected a document link for search %q", search)
	}

	return strings.TrimPrefix(href, "/documents/")
}

func assertAuthorDocuments(app *fiber.App, t *testing.T, authorSlug string, expectedResults int) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, "/authors/"+authorSlug, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	response, err := app.Test(req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, received %d", http.StatusOK, response.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}

	if actualResults := doc.Find("#list .list-group-item").Length(); actualResults != expectedResults {
		t.Errorf("Expected %d documents for author %q, got %d", expectedResults, authorSlug, actualResults)
	}
}
