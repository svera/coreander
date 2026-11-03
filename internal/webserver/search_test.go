package webserver_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/webserver"
	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
)

func TestLanguageFilterVisibility(t *testing.T) {
	cases := []struct {
		name      string
		files     []string
		languages []string
	}{
		{name: "no languages"},
		{
			name:      "one language",
			files:     []string{"testdata/library/metadata.epub"},
			languages: []string{"en"},
		},
		{
			name:      "multiple languages",
			files:     []string{"testdata/library/metadata.epub", "testdata/library/quijote.epub"},
			languages: []string{"en", "es"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var appFS afero.Fs = afero.NewMemMapFs()
			if len(tc.files) > 0 {
				appFS = loadFilesInMemoryFs(tc.files)
			}
			db := infrastructure.Connect(":memory:", 250)
			app := bootstrapApp(db, &infrastructure.NoEmail{}, appFS, webserver.Config{})

			for _, path := range []string{"/", "/search?type=documents&language=en"} {
				t.Run(path, func(t *testing.T) {
					req, err := http.NewRequest(http.MethodGet, path, nil)
					if err != nil {
						t.Fatal(err)
					}
					response, err := app.Test(req)
					if err != nil {
						t.Fatal(err)
					}
					defer response.Body.Close()
					if response.StatusCode != http.StatusOK {
						t.Fatalf("Expected status %d, received %d", http.StatusOK, response.StatusCode)
					}
					doc, err := goquery.NewDocumentFromReader(response.Body)
					if err != nil {
						t.Fatal(err)
					}

					filters := doc.Find(`select[name="language"]`)
					if len(tc.languages) <= 1 {
						if filters.Length() != 0 {
							t.Error("Expected no language dropdown with fewer than two languages")
						}
						return
					}

					selectors := []string{"#language"}
					if path != "/" {
						selectors = append(selectors, "#sidebar-language")
					}
					for _, selector := range selectors {
						filter := doc.Find(selector)
						if filter.Length() != 1 {
							t.Errorf("Expected language dropdown %s", selector)
							continue
						}
						if got := filter.Find("option").Length(); got != len(tc.languages)+1 {
							t.Errorf("Expected all-languages option and %d languages, got %d options", len(tc.languages), got)
						}
						for _, language := range tc.languages {
							if filter.Find(`option[value="`+language+`"]`).Length() != 1 {
								t.Errorf("Expected %s option in %s", language, selector)
							}
						}
					}
					if path != "/" && doc.Find(`#sidebar-language option[value="en"][selected]`).Length() != 1 {
						t.Error("Expected selected language to be preserved")
					}
				})
			}
		})
	}
}

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

func TestGoodReadsScoreSortOptions(t *testing.T) {
	app := bootstrapApp(infrastructure.Connect(":memory:", 250), &infrastructure.NoEmail{},
		loadDirInMemoryFs("testdata/library"), webserver.Config{})
	for _, path := range []string{
		"/search?type=documents&",
		"/authors/john-doe?",
		"/series/the-lord-of-the-rings?",
	} {
		for _, direction := range []string{"asc", "desc"} {
			for _, fragment := range []bool{false, true} {
				mode := "/full"
				if fragment {
					mode = "/fragment"
				}
				t.Run(path+direction+mode, func(t *testing.T) {
					selected := "review-score-" + direction
					req := mustGetRequest(t, path+"sort-by="+selected)
					if fragment {
						req.Header.Set("HX-Request", "true")
					}
					response, err := app.Test(req)
					if err != nil {
						t.Fatal(err)
					}
					defer response.Body.Close()
					if response.StatusCode != http.StatusOK {
						t.Fatalf("unexpected status %d", response.StatusCode)
					}
					doc, err := goquery.NewDocumentFromReader(response.Body)
					if err != nil {
						t.Fatal(err)
					}
					for _, value := range []string{"review-score-asc", "review-score-desc"} {
						if doc.Find(`#sort-by option[value="`+value+`"]`).Length() != 1 {
							t.Errorf("missing sort option %q", value)
						}
					}
					if doc.Find(`#sort-by option[value="`+selected+`"][selected]`).Length() != 1 {
						t.Errorf("expected selected option %q", selected)
					}
				})
			}
		}
	}
}

func TestReviewScoreFilter(t *testing.T) {
	db := infrastructure.Connect(":memory:", 250)
	app := bootstrapApp(db, &infrastructure.NoEmail{}, loadDirInMemoryFs("testdata/library"), webserver.Config{})

	response, err := app.Test(mustGetRequest(t, "/search?type=documents&review-score-from=4&review-score-to=5"))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, received %d", http.StatusOK, response.StatusCode)
	}
	doc, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "review-score-from", value: "4"},
		{name: "review-score-to", value: "5"},
	} {
		selects := doc.Find(`select[name="` + field.name + `"]`)
		if selects.Length() == 0 {
			t.Errorf("Expected Goodreads score filter %q", field.name)
			continue
		}
		selects.Each(func(_ int, selectBox *goquery.Selection) {
			if got, _ := selectBox.Find("option[selected]").Attr("value"); got != field.value {
				t.Errorf("Expected %s value %q, got %q", field.name, field.value, got)
			}
			options := selectBox.Find("option").Map(func(i int, option *goquery.Selection) string {
				value, _ := option.Attr("value")
				wantLabel := ""
				switch value {
				case "none":
					if strings.TrimSpace(option.Text()) == "" {
						t.Errorf("Expected a label for %s no-score option", field.name)
					}
					wantLabel = option.Text()
				case "1", "2", "3", "4", "5":
					wantLabel = value + " " + strings.Repeat("\u2605", i-1)
				}
				if option.Text() != wantLabel {
					t.Errorf("Expected %s option %q label %q, got %q", field.name, value, wantLabel, option.Text())
				}
				return value
			})
			if strings.Join(options, ",") != ",none,1,2,3,4,5" {
				t.Errorf("Expected blank, no-score, and scores 1 to 5 for %s, got %v", field.name, options)
			}
		})
		if doc.Find(`input[name="`+field.name+`"]`).Length() != 0 {
			t.Errorf("Expected no numeric input for %s", field.name)
		}

	}

	response, err = app.Test(mustGetRequest(t, "/search?type=documents&review-score-from=none"))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	doc, err = goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"review-score-from", "review-score-to"} {
		selected, _ := doc.Find(`select[name="` + name + `"] option[selected]`).Attr("value")
		if selected != "none" {
			t.Errorf("Expected no-score option selected for %s, got %q", name, selected)
		}
	}
}

func TestSearchResultsShowStoredDataSourceScore(t *testing.T) {
	for _, tt := range []struct {
		rating string
		want   string
	}{
		{rating: "3.82", want: "3.82"},
		{rating: "4.1", want: "4.10"},
		{rating: "5", want: "5.00"},
	} {
		t.Run(tt.rating, func(t *testing.T) {
			app, idx := newGoodReadsTestApp(t, nil)
			doc, err := idx.Document("linked")
			if err != nil {
				t.Fatal(err)
			}
			setDataSourceScore(t, idx, doc, tt.rating, "2,285")
			page, err := goquery.NewDocumentFromReader(strings.NewReader(getBody(t, app, "/search?type=documents&search=La")))
			if err != nil {
				t.Fatal(err)
			}
			rating := page.Find(`a[href="https://www.goodreads.com/book/show/12207444-la-reina-dragon"]`)
			if rating.Length() != 1 || strings.TrimSpace(rating.Text()) != tt.want {
				t.Fatalf("expected stored review score in search result, got %q", rating.Text())
			}
			if page.Find(`li:has(a[href="https://www.goodreads.com/book/show/12207444-la-reina-dragon"]) i.bi-star-fill`).Length() != 1 {
				t.Fatal("expected a star icon beside the review score")
			}
			if rating.Parent().Find("small").Text() != "(2,285 ratings)" {
				t.Fatal("expected rating count beside the review score")
			}
		})
	}
}

func TestSearchResultsSortByDataSourceScore(t *testing.T) {
	app, idx := newGoodReadsTestApp(t, &fakeBookSource{})
	scores := map[string]string{
		"linked":    "3.20",
		"unlinked":  "4.10",
		"unmatched": "4.80",
	}
	documents, err := idx.Search(index.SearchFields{}, 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, doc := range documents.Hits() {
		setDataSourceScore(t, idx, doc, scores[doc.Slug], "")
	}

	for _, tt := range []struct {
		direction string
		want      []string
	}{
		{direction: "asc", want: []string{"linked", "unlinked", "unmatched"}},
		{direction: "desc", want: []string{"unmatched", "unlinked", "linked"}},
	} {
		t.Run(tt.direction, func(t *testing.T) {
			body := getBody(t, app, "/search?type=documents&sort-by=review-score-"+tt.direction)
			page, err := goquery.NewDocumentFromReader(strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			page.Find("#list .list-group-item h2 a").Each(func(_ int, link *goquery.Selection) {
				href, _ := link.Attr("href")
				got = append(got, strings.TrimPrefix(href, "/documents/"))
			})
			if !slices.Equal(got, tt.want) {
				t.Fatalf("score sort %s: got %v, want %v", tt.direction, got, tt.want)
			}
		})
	}
}

func TestSearchResultsShowDataSourceScoreFromSearch(t *testing.T) {
	source := &fakeBookSource{results: []biblioreads.SearchResult{{
		Title: "Unlinked", Author: "Jane Doe", BookURL: "/book/show/1-unlinked",
		Rating: "4.20", RatingCount: "1,234",
	}}}
	app, idx := newGoodReadsTestApp(t, source)
	score := getBody(t, app, "/documents/unlinked/score")
	if !strings.Contains(score, "(1,234 ratings)") {
		t.Fatal("expected the search-derived rating count in the document score")
	}
	stored, err := idx.Document("unlinked")
	if err != nil || stored.DataSourceScoreCount != "1,234" {
		t.Fatalf("expected the search-derived count to be persisted, got %+v, %v", stored, err)
	}
	for _, path := range []string{
		"/search?type=documents&search=Unlinked",
		"/search?type=documents&search=Unlinked&l=es",
	} {
		page, err := goquery.NewDocumentFromReader(strings.NewReader(getBody(t, app, path)))
		if err != nil {
			t.Fatal(err)
		}
		rating := page.Find(`a[href="https://www.goodreads.com/book/show/1-unlinked"]`)
		if rating.Text() != "4.20" || !strings.Contains(rating.Parent().Find("small").Text(), "1,234") {
			t.Fatalf("expected the stored score and rating count in search results for %s", path)
		}
	}
	if len(source.ids) != 0 || len(source.queries) != 1 {
		t.Fatalf("expected one book search and no detail fetches, got queries %q and ids %q", source.queries, source.ids)
	}
}

func TestSearchResultsShowStoredZeroRatingAsNotScored(t *testing.T) {
	source := &fakeBookSource{results: []biblioreads.SearchResult{{
		Title: "Unlinked", Author: "Jane Doe", BookURL: "/book/show/1-unlinked", Rating: "0.00",
	}}}
	app, _ := newGoodReadsTestApp(t, source)
	getBody(t, app, "/documents/unlinked/goodreads")

	body := getBody(t, app, "/search?type=documents&search=Unlinked")
	page, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	row := page.Find("li").FilterFunction(func(_ int, item *goquery.Selection) bool {
		return strings.TrimSpace(item.Text()) == "Not scored" && item.Find("i.bi-star-fill").Length() == 1
	})
	if row.Length() == 0 || row.Find("a").Length() != 0 {
		t.Fatalf("expected stored zero rating to render as Not scored, got %q in %s", row.Text(), body)
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
