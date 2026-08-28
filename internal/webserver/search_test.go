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

// TestSearchSimilarToPreservesFilterAcrossReload checks that a "similar"
// search (from a document's "Similar documents" > "See all" link) keeps
// itself scoped to that document when the page reloads with extra filters
// applied - see the "similar" hidden input in document-search-filters.html,
// which regressed to being dropped on filter changes before this test was
// added - and that the Authors tab, which has no "similar to" concept, is
// hidden while in this mode.
func TestSearchSimilarToPreservesFilterAcrossReload(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	smtpMock := &infrastructure.SMTPMock{}
	appFS := loadDirInMemoryFs("testdata/library")

	app := bootstrapApp(db, smtpMock, appFS, webserver.Config{})

	req, err := http.NewRequest(http.MethodGet, "/search?type=documents&similar=john-doe-test-epub&language=en", nil)
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

	hiddenInputs := doc.Find(`input[name="similar"]`)
	if hiddenInputs.Length() == 0 {
		t.Fatal("Expected at least one hidden \"similar\" input to carry the filter across reloads, found none")
	}
	hiddenInputs.Each(func(_ int, s *goquery.Selection) {
		if val, _ := s.Attr("value"); val != "john-doe-test-epub" {
			t.Errorf("Expected hidden \"similar\" input to keep the reference document's slug, got %q", val)
		}
	})

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

// TestSearchSimilarToCloseButtonReturnsToRegularSearch checks that the close
// ("x") button on the "similar to" banner navigates to a regular search with
// "similar" removed but every other filter (language here) preserved, and
// that landing page has the search box and Authors tab back, and no banner.
func TestSearchSimilarToCloseButtonReturnsToRegularSearch(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	smtpMock := &infrastructure.SMTPMock{}
	appFS := loadDirInMemoryFs("testdata/library")

	app := bootstrapApp(db, smtpMock, appFS, webserver.Config{})

	req, err := http.NewRequest(http.MethodGet, "/search?type=documents&similar=john-doe-test-epub&language=en", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	response, err := app.Test(req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}

	doc, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}

	closeLink := doc.Find(".alert-secondary .btn-close")
	closeHref, ok := closeLink.Attr("href")
	if !ok || closeHref == "" {
		t.Fatal("Expected the banner's close button to have an href to navigate away from \"similar to\" mode")
	}
	if strings.Contains(closeHref, "similar=") {
		t.Errorf("Expected the close button's href to drop \"similar\", got %q", closeHref)
	}
	if !strings.Contains(closeHref, "language=en") {
		t.Errorf("Expected the close button's href to keep the language filter, got %q", closeHref)
	}

	req2, err := http.NewRequest(http.MethodGet, closeHref, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	response2, err := app.Test(req2)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err.Error())
	}
	if response2.StatusCode != http.StatusOK {
		t.Errorf("Expected status %d, received %d", http.StatusOK, response2.StatusCode)
	}

	doc2, err := goquery.NewDocumentFromReader(response2.Body)
	if err != nil {
		t.Fatal(err)
	}

	if doc2.Find(".alert-secondary").Length() != 0 {
		t.Error("Expected the \"similar to\" banner to be gone after following the close button")
	}
	if doc2.Find(`#sidebar-search, #searchbox-offcanvas`).Length() == 0 {
		t.Error("Expected the free-text search box to be back after following the close button")
	}
	if doc2.Find(`[data-search-type-tab="authors"]`).Length() == 0 {
		t.Error("Expected the Authors tab to be back after following the close button")
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
