package webserver_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/blevesearch/bleve/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/metadata"
	"github.com/svera/coreander/v5/internal/reviews"
	"github.com/svera/coreander/v5/internal/servicelinks"
	"github.com/svera/coreander/v5/internal/webserver"
	"github.com/svera/coreander/v5/internal/webserver/controller/author"
	"github.com/svera/coreander/v5/internal/webserver/controller/document"
	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
	"github.com/svera/coreander/v5/internal/webserver/model"
)

type fakeBookSource struct {
	mu          sync.Mutex
	ids         []string
	queries     []string
	book        biblioreads.BookResponse
	results     []biblioreads.SearchResult
	err         error
	searchErr   error
	reviews     biblioreads.ReviewsResponse
	reviewErr   error
	reviewCalls []reviewCall
}

func TestDocumentReviewsCanSelectSupportedLanguage(t *testing.T) {
	source := &fakeBookSource{book: biblioreads.BookResponse{
		Rating: "4", ResourceID: "work-id", LegacyBookID: 1,
	}}
	app, _ := newGoodReadsTestApp(t, source)
	req := mustGetRequest(t, "/documents/linked/reviews?language=fr")
	req.Header.Set("Accept-Language", "es")
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
	if len(source.reviewCalls) != 1 || source.reviewCalls[0].languageCode != "fr" {
		t.Fatalf("expected Goodreads reviews in French, got %+v", source.reviewCalls)
	}
	if page.Find("h2").Text() != "Reseñas de usuarios de Goodreads" {
		t.Fatalf("expected page language to remain Spanish while selecting French reviews, got %q", page.Find("h2").Text())
	}
	selectBox := page.Find(`#review-language-linked`)
	if selectBox.Length() != 1 || selectBox.Find(`option[value="fr"][selected]`).Length() != 1 {
		html, _ := selectBox.Html()
		t.Fatalf("expected the review-language selector to retain the selected language: %q", html)
	}
	if indicator, _ := selectBox.Attr("hx-indicator"); indicator != "#document-reviews-linked" ||
		page.Find(`#document-reviews-linked .document-reviews-loading > div`).Length() != 2 {
		t.Fatal("expected a review skeleton indicator for language changes")
	}
	for _, language := range webserver.SupportedLanguages() {
		if selectBox.Find(`option[value="`+language+`"]`).Length() != 1 {
			t.Errorf("expected supported language %q in the review selector", language)
		}
	}
}

func TestDocumentReviewsRejectUnsupportedLanguage(t *testing.T) {
	source := &fakeBookSource{book: biblioreads.BookResponse{
		Rating: "4", ResourceID: "work-id", LegacyBookID: 1,
	}}
	app, _ := newGoodReadsTestApp(t, source)
	response, err := app.Test(mustGetRequest(t, "/documents/linked/reviews?language=zz"))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	mustReturnStatus(response, http.StatusBadRequest, t)
	if len(source.reviewCalls) != 0 {
		t.Fatalf("unsupported language must not be sent to Goodreads, got %+v", source.reviewCalls)
	}
}

type fakeReviewProvider struct {
	score           reviews.Score
	page            reviews.Page
	scoreErr        error
	reviewsErr      error
	scoreSlugs      []string
	reviewSlugs     []string
	reviewLanguages []string
}

func (f *fakeReviewProvider) Score(_ context.Context, document index.Document) (reviews.Score, error) {
	f.scoreSlugs = append(f.scoreSlugs, document.Slug)
	return f.score, f.scoreErr
}

func (f *fakeReviewProvider) Reviews(_ context.Context, document index.Document, language string) (reviews.Page, error) {
	f.reviewSlugs = append(f.reviewSlugs, document.Slug)
	f.reviewLanguages = append(f.reviewLanguages, language)
	return f.page, f.reviewsErr
}

func TestDocumentReviewsUILanguage(t *testing.T) {
	for _, tt := range []struct {
		name   string
		query  string
		cookie string
		header string
		want   string
	}{
		{name: "unsupported header falls back to English", header: "zz", want: "en"},
		{name: "header", header: "fr", want: "fr"},
		{name: "cookie overrides header", cookie: "es", header: "en", want: "es"},
		{name: "query overrides cookie", query: "?l=de", cookie: "es", header: "en", want: "de"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			source := &fakeBookSource{book: biblioreads.BookResponse{
				Rating: "4", ResourceID: "work-id", LegacyBookID: 1,
			}}
			app, _ := newGoodReadsTestApp(t, source)
			req := mustGetRequest(t, "/documents/linked/reviews"+tt.query)
			if tt.header != "" {
				req.Header.Set("Accept-Language", tt.header)
			}
			if tt.cookie != "" {
				req.AddCookie(&http.Cookie{Name: "locale", Value: tt.cookie})
			}
			response, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			mustReturnStatus(response, http.StatusOK, t)
			if len(source.reviewCalls) != 1 || source.reviewCalls[0].languageCode != tt.want {
				t.Fatalf("expected BiblioReads language %q, got %+v", tt.want, source.reviewCalls)
			}
		})
	}
}

func TestDocumentAlternativeReviewProvider(t *testing.T) {
	provider := &fakeReviewProvider{
		score: reviews.Score{Source: "Another service", Value: "4.5", Count: "123", URL: "https://example.org/books/book-1"},
		page: reviews.Page{
			Source: "Another service", URL: "https://example.org/books/book-1", Available: true,
			Reviews: []reviews.Review{{
				Reviewer: "Reader", Rating: 5, Text: "<p>Excellent</p><script>unsafe()</script>",
				CreatedAt: time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC), URL: "https://example.org/reviews/review-1",
			}},
		},
	}
	app, _ := newReviewProviderTestApp(t, nil, provider)
	// This document has a cached Goodreads non-match; it must still use the chosen provider.
	detail := getBody(t, app, "/documents/unmatched")
	if goodReadsPlaceholders(t, detail).Length() != 1 || len(provider.scoreSlugs) != 0 || len(provider.reviewSlugs) != 0 {
		t.Fatal("detail must defer lookups to the selected provider")
	}
	for _, path := range []string{"/documents/unmatched/score", "/documents/unmatched/goodreads"} {
		body := getBody(t, app, path)
		page, err := goquery.NewDocumentFromReader(strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		link := page.Find(`li a[href="https://example.org/books/book-1"]`)
		if link.Text() != "4.5" || !strings.Contains(body, "Another service rating") || strings.Contains(body, "goodreads.com") {
			t.Fatalf("score must use the selected provider's labels, value and URL: %s", body)
		}
	}
	body := getBody(t, app, "/documents/unmatched/reviews")
	page, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if page.Find("h2").Text() != "Another service user reviews" ||
		page.Find(`a[href="https://example.org/reviews/review-1"] time[datetime="2026-10-30"]`).Length() != 1 ||
		page.Find("article strong").Text() != "Reader" ||
		page.Find("i.bi-star-fill").Length() != 5 || page.Find("script").Length() != 0 {
		t.Fatalf("reviews must render normalized provider data safely: %s", body)
	}
	if len(provider.scoreSlugs) != 2 || len(provider.reviewSlugs) != 1 || provider.reviewSlugs[0] != "unmatched" {
		t.Fatal("expected calls through the provider interface")
	}
	if len(provider.reviewLanguages) != 1 || provider.reviewLanguages[0] != "en" {
		t.Fatalf("expected the current UI language to reach alternative providers, got %q", provider.reviewLanguages)
	}
}

func TestDocumentScoreMarksZeroWithoutValuationsAsNotScored(t *testing.T) {
	for _, tt := range []struct {
		name       string
		count      string
		wantScore  string
		wantScored bool
	}{
		{name: "zero valuations", count: "0", wantScore: "Not scored"},
		{name: "valuations exist", count: "650", wantScore: "0.00", wantScored: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			provider := &fakeReviewProvider{
				score: reviews.Score{
					Source: "Goodreads", Value: "0.00", Count: tt.count,
					URL: "https://www.goodreads.com/book/show/1",
				},
			}
			app, _ := newReviewProviderTestApp(t, nil, provider)
			page, err := goquery.NewDocumentFromReader(strings.NewReader(getBody(t, app, "/documents/unmatched/score")))
			if err != nil {
				t.Fatal(err)
			}
			row := page.Find("li")
			if tt.wantScored {
				if got := strings.TrimSpace(row.Find("a").Text()); got != tt.wantScore {
					t.Fatalf("expected score %q, got %q", tt.wantScore, got)
				}
				return
			}
			if got := strings.TrimSpace(row.Text()); got != tt.wantScore || row.Find("a").Length() != 0 {
				t.Fatalf("expected unlinked %q label, got %q", tt.wantScore, got)
			}
		})
	}
}

type reviewCall struct {
	resourceID    string
	legacyBookID  int
	nextPageToken string
	languageCode  string
}

func (f *fakeBookSource) Reviews(_ context.Context, resourceID string, legacyBookID int, nextPageToken, languageCode string) (biblioreads.ReviewsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reviewCalls = append(f.reviewCalls, reviewCall{resourceID, legacyBookID, nextPageToken, languageCode})
	return f.reviews, f.reviewErr
}

func (f *fakeBookSource) SearchBooks(_ context.Context, query string) (biblioreads.SearchBooksResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, query)
	return biblioreads.SearchBooksResponse{Result: f.results}, f.searchErr
}

func (f *fakeBookSource) Book(_ context.Context, id string) (biblioreads.BookResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ids = append(f.ids, id)
	return f.book, f.err
}

func newGoodReadsTestApp(t *testing.T, source *fakeBookSource) (*fiber.App, *index.BleveIndexer) {
	t.Helper()
	return newReviewProviderTestApp(t, source, nil)
}

func newReviewProviderTestApp(t *testing.T, source *fakeBookSource, provider document.ReviewProvider) (*fiber.App, *index.BleveIndexer) {
	t.Helper()
	return newServiceProvidersTestApp(t, source, provider, servicelinks.Goodreads{})
}

func newServiceProvidersTestApp(t *testing.T, source *fakeBookSource, provider document.ReviewProvider, links author.LinkProvider) (*fiber.App, *index.BleveIndexer) {
	t.Helper()
	db := infrastructure.Connect(":memory:", 250)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	documents, err := bleve.NewMemOnly(index.CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}
	authors, err := bleve.NewMemOnly(index.CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}
	appFS := afero.NewMemMapFs()
	idxCfg := index.Config{}
	if source != nil {
		idxCfg.GoodReadsSource = source
	}
	idx := index.NewBleve(documents, authors, appFS, testLibraryDir, nil, idxCfg)
	if source != nil {
		provider = reviews.NewBiblioReads(source, idx)
	}
	t.Cleanup(func() { _ = idx.Close() })

	for _, doc := range []index.Document{
		{
			ID: "linked.epub", Slug: "linked",
			Metadata:             metadata.Metadata{Title: "La Reina Dragon", Format: "EPUB"},
			GoodReadsBookURL:     "/book/show/12207444-la-reina-dragon",
			GoodReadsRating:      "3.82",
			GoodReadsRatingValue: 3.82,
			GoodReadsEnriched:    true,
		},
		{
			ID: "unlinked.epub", Slug: "unlinked",
			Metadata:     metadata.Metadata{Title: "Unlinked", Authors: []string{"Jane Doe", "John Doe"}, Format: "EPUB"},
			AuthorsSlugs: []string{"jane-doe", "john-doe"},
		},
		{
			ID: "unmatched.epub", Slug: "unmatched",
			Metadata:          metadata.Metadata{Title: "Unmatched", Format: "EPUB"},
			GoodReadsEnriched: true,
		},
	} {
		if err := documents.Index(doc.ID, doc); err != nil {
			t.Fatal(err)
		}
	}

	cfg := defaultTestConfig()
	sender := &infrastructure.NoEmail{}
	controllers := webserver.SetupControllers(cfg, db, idx, sender, appFS, nil, provider, links)
	app := webserver.New(cfg, controllers, sender, idx, &model.UserRepository{DB: db})
	t.Cleanup(func() { _ = app.Shutdown() })
	return app, idx
}

func TestSearchResultsShowStoredGoodReadsRating(t *testing.T) {
	app, _ := newGoodReadsTestApp(t, nil)
	page, err := goquery.NewDocumentFromReader(strings.NewReader(getBody(t, app, "/search?type=documents&search=La")))
	if err != nil {
		t.Fatal(err)
	}
	rating := page.Find(`a[href="https://www.goodreads.com/book/show/12207444-la-reina-dragon"]`)
	if rating.Length() != 1 || strings.TrimSpace(rating.Text()) != "3.82" {
		t.Fatalf("expected stored Goodreads rating in search result, got %q", rating.Text())
	}
	if page.Find(`li:has(a[href="https://www.goodreads.com/book/show/12207444-la-reina-dragon"]) i.bi-star-fill`).Length() != 1 {
		t.Fatal("expected a star icon beside the Goodreads score")
	}
}

func TestDocumentDetailShowsStoredZeroRatingAsNotScored(t *testing.T) {
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

func getBody(t *testing.T, app *fiber.App, path string) string {
	t.Helper()
	req := mustGetRequest(t, path)
	req.Header.Set("Accept-Language", "en")
	response, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("%s: status = %d, want %d", path, response.StatusCode, http.StatusOK)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func goodReadsPlaceholders(t *testing.T, html string) *goquery.Selection {
	t.Helper()
	page, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	return page.Find(`li[hx-get$="/score"]`)
}

func TestDocumentDetailGoodReadsRating(t *testing.T) {
	source := &fakeBookSource{book: biblioreads.BookResponse{
		Rating: "3.82", RatingCount: "2,285",
		ResourceID: "work-id", LegacyBookID: 12207444,
	}}
	app, idx := newGoodReadsTestApp(t, source)

	placeholder := goodReadsPlaceholders(t, getBody(t, app, "/documents/linked"))
	if placeholder.Length() != 1 {
		t.Fatalf("expected one GoodReads placeholder, got %d", placeholder.Length())
	}
	if url, _ := placeholder.Attr("hx-get"); url != "/documents/linked/score" {
		t.Errorf("unexpected placeholder URL %q", url)
	}
	if placeholder.ParentsFiltered(`[id="document-metadata-linked"]`).Length() != 1 {
		t.Error("expected the placeholder inside the document metadata list")
	}

	page, err := goquery.NewDocumentFromReader(strings.NewReader(getBody(t, app, "/documents/linked/goodreads")))
	if err != nil {
		t.Fatal(err)
	}
	link := page.Find(`li a[href="https://www.goodreads.com/book/show/12207444-la-reina-dragon"]`)
	if page.Find("li i.bi-star-fill").Length() != 1 {
		t.Error("expected the rating row to retain its star icon")
	}
	if strings.TrimSpace(link.Text()) != "3.82" {
		t.Errorf("expected rating link with 3.82, got %q", link.Text())
	}
	if !strings.Contains(page.Find("li").Text(), "2,285") {
		t.Errorf("expected rating count in %q", page.Find("li").Text())
	}
	if len(source.ids) != 1 || source.ids[0] != "12207444-la-reina-dragon" {
		t.Errorf("expected Book to be called with the stored id, got %q", source.ids)
	}
	if len(source.queries) != 0 {
		t.Errorf("expected no search for a linked document, got %q", source.queries)
	}
	stored, err := idx.Document("linked")
	if err != nil {
		t.Fatal(err)
	}
	if stored.GoodReadsResourceID != source.book.ResourceID || stored.GoodReadsLegacyBookID != source.book.LegacyBookID {
		t.Fatalf("expected book identifiers to be stored, got %+v", stored)
	}
}

func TestDocumentGoodReadsEditor(t *testing.T) {
	source := &fakeBookSource{book: biblioreads.BookResponse{Rating: "3.82"}}
	app, _ := newGoodReadsTestApp(t, source)
	adminCookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name   string
		cookie *http.Cookie
		want   int
	}{
		{name: "anonymous", cookie: &http.Cookie{}},
		{name: "administrator", cookie: adminCookie, want: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response, err := getRequest(tt.cookie, app, "/documents/linked/score", t)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			mustReturnStatus(response, http.StatusOK, t)
			page, err := goquery.NewDocumentFromReader(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			control := page.Find(`li a[data-bs-toggle="collapse"][href="#edit-goodreads-linked"]`)
			if control.Length() != tt.want {
				t.Fatalf("expected %d edit links, got %d", tt.want, control.Length())
			}
			if tt.want == 0 {
				if page.Find(`input[name="bookURL"]`).Length() != 0 {
					t.Fatal("anonymous readers must not receive the editor")
				}
				return
			}
			if control.Text() != "Edit Goodreads link" ||
				page.Find(`#edit-goodreads-linked.collapse:not(.show)`).Length() != 1 ||
				page.Find(`form[hx-put="/documents/linked/goodreads"][hx-target="this"]`).Length() != 1 ||
				page.Find(`label[for="goodreads-book-url-linked"]`).Length() != 1 {
				t.Fatal("expected accessible, initially collapsed Goodreads editor beside the score")
			}
			if value, _ := page.Find(`input[name="bookURL"]`).Attr("value"); value != "https://www.goodreads.com/book/show/12207444-la-reina-dragon" {
				t.Fatalf("editor must be prefilled with the current link, got %q", value)
			}
		})
	}

	source.err = biblioreads.ErrUnavailable
	response, err := getRequest(adminCookie, app, "/documents/linked/score", t)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	page, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page.Find("li").Text(), "Unable to load Goodreads score") ||
		page.Find(`input[name="bookURL"]`).Length() != 1 {
		t.Fatal("an unavailable score must not prevent administrators from correcting the link")
	}
}

func TestDocumentUpdateGoodReads(t *testing.T) {
	source := &fakeBookSource{book: biblioreads.BookResponse{
		Rating: "3.82", ResourceID: "old-work", LegacyBookID: 12207444,
	}}
	app, idx := newGoodReadsTestApp(t, source)
	getBody(t, app, "/documents/linked/score")
	adminCookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatal(err)
	}
	response, err := putRequest(url.Values{
		"bookURL": {"https://www.goodreads.com/book/show/1368314.El_Cercano_Oriente?from_search=true"},
	}, adminCookie, app, "/documents/linked/goodreads", t)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	mustReturnStatus(response, http.StatusNoContent, t)
	if response.Header.Get("HX-Refresh") != "true" {
		t.Fatal("successful correction must reload the detail page")
	}
	stored, err := idx.Document("linked")
	if err != nil {
		t.Fatal(err)
	}
	if stored.GoodReadsBookURL != "/book/show/1368314.El_Cercano_Oriente" ||
		stored.GoodReadsRating != "" || stored.GoodReadsRatingValue != 0 ||
		stored.GoodReadsResourceID != "" || stored.GoodReadsLegacyBookID != 0 ||
		!stored.GoodReadsEnriched {
		t.Fatalf("expected correction persisted without stale rating/review identifiers: %+v", stored)
	}
	source.book = biblioreads.BookResponse{Rating: "4.2", ResourceID: "new-work", LegacyBookID: 1368314}
	body := getBody(t, app, "/documents/linked/score")
	if !strings.Contains(body, `href="https://www.goodreads.com/book/show/1368314.El_Cercano_Oriente"`) ||
		source.ids[len(source.ids)-1] != "1368314.El_Cercano_Oriente" {
		t.Fatal("subsequent scores must use the corrected book")
	}
	getBody(t, app, "/documents/linked/reviews")
	if len(source.reviewCalls) != 1 || source.reviewCalls[0].resourceID != "new-work" ||
		source.reviewCalls[0].legacyBookID != 1368314 {
		t.Fatalf("subsequent reviews must use the corrected book: %+v", source.reviewCalls)
	}
}

func TestDocumentUpdateGoodReadsValidation(t *testing.T) {
	app, idx := newGoodReadsTestApp(t, nil)
	adminCookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", "https://example.org/book/show/123", "https://www.goodreads.com/author/show/16667"} {
		response, err := putRequest(url.Values{"bookURL": {value}}, adminCookie, app, "/documents/linked/goodreads", t)
		if err != nil {
			t.Fatal(err)
		}
		mustReturnStatus(response, http.StatusUnprocessableEntity, t)
		page, err := goquery.NewDocumentFromReader(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if page.Find(`form#goodreads-link-form-linked input[aria-invalid="true"]`).Length() != 1 ||
			page.Find(`[role="alert"]`).Text() != "Enter a valid Goodreads book URL." {
			t.Fatal("invalid URLs must render inline validation feedback")
		}
		if got, _ := page.Find(`input[name="bookURL"]`).Attr("value"); got != value {
			t.Fatalf("invalid input should be retained for correction, got %q", got)
		}
		stored, err := idx.Document("linked")
		if err != nil || stored.GoodReadsBookURL != "/book/show/12207444-la-reina-dragon" || stored.GoodReadsRating != "3.82" {
			t.Fatalf("invalid input must not change the stored match: %+v, %v", stored, err)
		}
	}
	response, err := putRequest(url.Values{
		"bookURL": {"https://www.goodreads.com/book/show/1368314"},
	}, adminCookie, app, "/documents/missing/goodreads", t)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	mustReturnStatus(response, http.StatusNotFound, t)
}

func TestDocumentDetailUpdatesStoredGoodReadsRating(t *testing.T) {
	source := &fakeBookSource{book: biblioreads.BookResponse{
		Rating: "4.12", ResourceID: "work-id", LegacyBookID: 12207444,
	}}
	app, idx := newGoodReadsTestApp(t, source)

	page, err := goquery.NewDocumentFromReader(strings.NewReader(getBody(t, app, "/documents/linked/goodreads")))
	if err != nil {
		t.Fatal(err)
	}
	link := page.Find(`li a[href="https://www.goodreads.com/book/show/12207444-la-reina-dragon"]`)
	if got := strings.TrimSpace(link.Text()); got != "4.12" {
		t.Fatalf("expected latest Goodreads rating 4.12, got %q", got)
	}
	stored, err := idx.Document("linked")
	if err != nil {
		t.Fatal(err)
	}
	if stored.GoodReadsRating != "4.12" || stored.GoodReadsRatingValue != 4.12 {
		t.Fatalf("expected latest rating and searchable value to be stored, got %+v", stored)
	}
	results, err := idx.Search(index.SearchFields{GoodReadsRatingFrom: 4.1, GoodReadsRatingTo: 4.2}, 1, 10)
	if err != nil || results.TotalHits() != 1 {
		t.Fatalf("expected document to match updated rating range, got %d results and error %v", results.TotalHits(), err)
	}
}

func TestDocumentDetailKeepsStoredGoodReadsRatingWhenScraperReturnsZero(t *testing.T) {
	source := &fakeBookSource{book: biblioreads.BookResponse{
		Rating: "0.00", RatingCount: "650", ResourceID: "work-id", LegacyBookID: 12207444,
	}}
	app, idx := newGoodReadsTestApp(t, source)

	page, err := goquery.NewDocumentFromReader(strings.NewReader(getBody(t, app, "/documents/linked/goodreads")))
	if err != nil {
		t.Fatal(err)
	}
	link := page.Find(`li a[href="https://www.goodreads.com/book/show/12207444-la-reina-dragon"]`)
	if got := strings.TrimSpace(link.Text()); got != "3.82" {
		t.Fatalf("expected known Goodreads rating to be retained, got %q", got)
	}
	stored, err := idx.Document("linked")
	if err != nil {
		t.Fatal(err)
	}
	if stored.GoodReadsRating != "3.82" || stored.GoodReadsRatingValue != 3.82 {
		t.Fatalf("zero scraper rating must not replace indexed Goodreads score, got %+v", stored)
	}
}

func TestDocumentDetailGoodReadsSearchesUnlinkedDocuments(t *testing.T) {
	source := &fakeBookSource{
		book:    biblioreads.BookResponse{Rating: "4.20", RatingCount: "10", ResourceID: "work-id", LegacyBookID: 1},
		results: []biblioreads.SearchResult{{Title: "Unlinked", Author: "Jane Doe", BookURL: "/book/show/1-unlinked", AuthorURL: "/author/show/1.Jane_Doe", Rating: "4.10"}, {BookURL: "/book/show/2-other", Rating: "5.00"}},
	}
	app, idx := newGoodReadsTestApp(t, source)

	if got := goodReadsPlaceholders(t, getBody(t, app, "/documents/unlinked")).Length(); got != 1 {
		t.Fatalf("expected a placeholder for a document not looked up yet, got %d", got)
	}

	page, err := goquery.NewDocumentFromReader(strings.NewReader(getBody(t, app, "/documents/unlinked/goodreads")))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(page.Find(`li a[href="https://www.goodreads.com/book/show/1-unlinked"]`).Text()); got != "4.10" {
		t.Errorf("expected rating link with 4.10, got %q", got)
	}
	if len(source.queries) != 1 || source.queries[0] != "Unlinked Jane Doe" {
		t.Errorf("expected a single search with title and first author, got %q", source.queries)
	}
	if len(source.ids) != 0 {
		t.Errorf("expected search rating without a book-scraper call, got %q", source.ids)
	}
	if page.Find("small").Length() != 0 {
		t.Error("search response should not display a rating count")
	}

	stored, err := idx.Document("unlinked")
	if err != nil {
		t.Fatal(err)
	}
	if stored.GoodReadsBookURL != "/book/show/1-unlinked" || !stored.GoodReadsEnriched {
		t.Errorf("expected the found link to be stored, got %+v", stored)
	}
	author, err := idx.Author("jane-doe", "")
	if err != nil || author.GoodReadsAuthorURL != "/author/show/1.Jane_Doe" || author.DocumentCount != 1 {
		t.Fatalf("expected missing author entry created with URL and document count, got %+v, %v", author, err)
	}
	if stored.GoodReadsResourceID != "" || stored.GoodReadsLegacyBookID != 0 {
		t.Fatalf("search-only lookup must not populate scraper identifiers, got %+v", stored)
	}

	if err := idx.EnrichGoodReads(10); err != nil {
		t.Fatal(err)
	}
	page, err = goquery.NewDocumentFromReader(strings.NewReader(getBody(t, app, "/documents/unlinked/goodreads")))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(page.Find(`li a[href="https://www.goodreads.com/book/show/1-unlinked"]`).Text()); got != "4.20" {
		t.Errorf("expected later request to use book-scraper rating, got %q", got)
	}
	if len(source.queries) != 1 {
		t.Errorf("expected the stored link to be reused without searching again, got %q", source.queries)
	}
	if len(source.ids) != 1 || source.ids[0] != "1-unlinked" {
		t.Errorf("expected later request to scrape the stored book URL, got %q", source.ids)
	}
}

func TestDocumentDetailGoodReadsSkipsUnmatchedDocuments(t *testing.T) {
	source := &fakeBookSource{book: biblioreads.BookResponse{Rating: "3.82"}}
	app, _ := newGoodReadsTestApp(t, source)
	if got := goodReadsPlaceholders(t, getBody(t, app, "/documents/unmatched")).Length(); got != 1 {
		t.Errorf("expected an async score placeholder, got %d", got)
	}
	for _, path := range []string{"/documents/unmatched/score", "/documents/unmatched/goodreads"} {
		page, err := goquery.NewDocumentFromReader(strings.NewReader(getBody(t, app, path)))
		if err != nil {
			t.Fatal(err)
		}
		score := page.Find("li").FilterFunction(func(_ int, item *goquery.Selection) bool {
			return item.Find("i.bi-star-fill").Length() == 1
		})
		if strings.TrimSpace(score.Text()) != "Not scored" || score.Find("a").Length() != 0 {
			t.Errorf("%s: expected unlinked no-score row, got %q", path, score.Text())
		}
	}
	if len(source.queries) != 0 || len(source.ids) != 0 {
		t.Errorf("expected no calls, got queries %q and ids %q", source.queries, source.ids)
	}
}

func TestDocumentDetailGoodReadsRatingRendersNothingWhenUnavailable(t *testing.T) {
	for name, source := range map[string]*fakeBookSource{
		"error": {err: errors.New("unavailable")},
	} {
		t.Run(name, func(t *testing.T) {
			app, _ := newGoodReadsTestApp(t, source)
			if body := strings.TrimSpace(getBody(t, app, "/documents/linked/goodreads")); body != "" {
				t.Errorf("expected empty response, got %q", body)
			}
		})
	}
}

func TestDocumentDetailGoodReadsIdentifiersWithoutRating(t *testing.T) {
	source := &fakeBookSource{book: biblioreads.BookResponse{ResourceID: "work-id", LegacyBookID: 12207444}}
	app, idx := newGoodReadsTestApp(t, source)
	page, err := goquery.NewDocumentFromReader(strings.NewReader(getBody(t, app, "/documents/linked/goodreads")))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(page.Find("li").Text()) != "Not scored" || page.Find("h2").Text() != "Goodreads user reviews" {
		t.Fatal("expected no score and a reviews heading for a matched book without a rating")
	}
	stored, err := idx.Document("linked")
	if err != nil {
		t.Fatal(err)
	}
	if stored.GoodReadsResourceID != "work-id" || stored.GoodReadsLegacyBookID != 12207444 {
		t.Fatalf("expected identifiers even without a rating, got %+v", stored)
	}

	for _, book := range []biblioreads.BookResponse{
		{},
		{ResourceID: "partial-work"},
		{LegacyBookID: 2},
	} {
		source.book = book
		getBody(t, app, "/documents/linked/goodreads")
		stored, err = idx.Document("linked")
		if err != nil {
			t.Fatal(err)
		}
		if stored.GoodReadsResourceID != "work-id" || stored.GoodReadsLegacyBookID != 12207444 {
			t.Fatalf("incomplete response overwrote stored identifiers: %+v", stored)
		}
	}
}

func TestDocumentDetailGoodReadsDisabled(t *testing.T) {
	app, _ := newGoodReadsTestApp(t, nil)
	for _, slug := range []string{"linked", "unlinked", "unmatched"} {
		if got := goodReadsPlaceholders(t, getBody(t, app, "/documents/"+slug)).Length(); got != 0 {
			t.Errorf("%s: expected no placeholder when GoodReads is disabled, got %d", slug, got)
		}
		if body := strings.TrimSpace(getBody(t, app, "/documents/"+slug+"/goodreads")); body != "" {
			t.Errorf("%s: expected empty response, got %q", slug, body)
		}
		if body := getBody(t, app, "/documents/"+slug); strings.Contains(body, "bi-star-fill") {
			t.Errorf("%s: expected no score row when integration is disabled", slug)
		}
		if body := getBody(t, app, "/documents/"+slug); strings.Contains(body, `id="document-reviews-`) {
			t.Errorf("%s: expected no reviews target when integration is disabled", slug)
		}
	}
}

func TestDocumentDetailGoodReadsSearchDoesNotScrape(t *testing.T) {
	for _, tt := range []struct {
		name     string
		results  []biblioreads.SearchResult
		err      error
		queries  int
		enriched bool
		noScore  bool
	}{
		{
			name:    "matching book without rating",
			results: []biblioreads.SearchResult{{Title: "Unlinked", Author: "Jane Doe", BookURL: "/book/show/1-unlinked"}},
			queries: 1, enriched: true,
		},
		{name: "no match", queries: 2, enriched: true, noScore: true},
		{name: "unavailable search", err: biblioreads.ErrUnavailable, queries: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			source := &fakeBookSource{
				results: tt.results, searchErr: tt.err,
				book: biblioreads.BookResponse{Rating: "5.00"},
			}
			app, idx := newGoodReadsTestApp(t, source)
			body := strings.TrimSpace(getBody(t, app, "/documents/unlinked/goodreads"))
			if tt.noScore {
				page, err := goquery.NewDocumentFromReader(strings.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				if strings.TrimSpace(page.Find("li").Text()) != "Not scored" || page.Find("a").Length() != 0 {
					t.Errorf("expected no-score row without a link, got %q", body)
				}
			} else if tt.enriched && len(tt.results) > 0 {
				page, err := goquery.NewDocumentFromReader(strings.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				if page.Find("#document-reviews-unlinked h2").Text() != "Goodreads user reviews" ||
					page.Find("#document-reviews-unlinked[hx-get]").Length() != 0 {
					t.Fatal("a match without a rating must show the heading without fetching reviews")
				}
			} else if body != "" {
				t.Errorf("expected no rating from search, got %q", body)
			}
			if len(source.ids) != 0 || len(source.queries) != tt.queries {
				t.Errorf("expected %d searches and no book-scraper calls, got queries %q and ids %q", tt.queries, source.queries, source.ids)
			}
			stored, err := idx.Document("unlinked")
			if err != nil || stored.GoodReadsEnriched != tt.enriched {
				t.Fatalf("unexpected stored lookup status: %+v, %v", stored, err)
			}
		})
	}
}

func TestDocumentDetailGoodReadsNoScoreTranslations(t *testing.T) {
	source := &fakeBookSource{}
	app, _ := newGoodReadsTestApp(t, source)
	for _, locale := range []struct {
		lang string
		text string
	}{
		{"en", "Not scored"},
		{"es", "Sin puntuar"},
		{"de", "Nicht bewertet"},
		{"fr", "Non noté"},
		{"ru", "Не оценено"},
	} {
		for _, path := range []string{"/documents/unmatched/score", "/documents/unmatched/goodreads"} {
			t.Run(locale.lang+path, func(t *testing.T) {
				req := mustGetRequest(t, path)
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
				score := page.Find("li").FilterFunction(func(_ int, item *goquery.Selection) bool {
					return item.Find("i.bi-star-fill").Length() == 1
				})
				if strings.TrimSpace(score.Text()) != locale.text || score.Find("a").Length() != 0 {
					t.Fatalf("expected localized no-score row %q, got %q", locale.text, score.Text())
				}
			})
		}
	}
	if len(source.queries) != 0 || len(source.ids) != 0 {
		t.Fatal("cached unmatched documents must not trigger Goodreads requests")
	}
}

func TestDocumentDetailGoodReadsReviewsLazyPlaceholder(t *testing.T) {
	source := &fakeBookSource{book: biblioreads.BookResponse{
		Rating: "3.82", ResourceID: "work-id", LegacyBookID: 12207444,
	}}
	app, _ := newGoodReadsTestApp(t, source)
	page, err := goquery.NewDocumentFromReader(strings.NewReader(getBody(t, app, "/documents/linked")))
	if err != nil {
		t.Fatal(err)
	}
	target := page.Find("#document-reviews-linked")
	if target.Length() != 1 || target.Is("[hx-get]") {
		t.Fatal("expected an inactive reviews target before the rating loads")
	}
	if target.Find("h2").Text() != "Goodreads user reviews" {
		t.Fatal("expected the reviews heading before the score loads")
	}
	if !target.Is(".document-detail-content > :last-child") {
		t.Fatal("reviews must appear at the bottom of the document content")
	}
	page, err = goquery.NewDocumentFromReader(strings.NewReader(getBody(t, app, "/documents/linked/goodreads")))
	if err != nil {
		t.Fatal(err)
	}
	placeholder := page.Find(`#document-reviews-linked[hx-get="/documents/linked/reviews"][hx-trigger="intersect once"][hx-swap-oob="outerHTML"]`)
	if placeholder.Length() != 1 {
		t.Fatal("expected successful rating to enable reviews on intersection only")
	}
	if placeholder.Find("h2").Text() != "Goodreads user reviews" || placeholder.Find(`[aria-hidden="true"] h2`).Length() != 0 {
		t.Fatal("the reviews heading must remain visible and accessible while reviews load")
	}
	skeletons := placeholder.Find(".border-top")
	if skeletons.Length() != 2 || placeholder.Find(`div[aria-hidden="true"]`).Length() != 1 {
		t.Fatal("expected two decorative review-shaped skeletons")
	}
	skeletons.Each(func(i int, skeleton *goquery.Selection) {
		if skeleton.Find(".fs-5 .placeholder").Length() != 1 ||
			skeleton.Find("i.bi-star").Length() != 5 ||
			skeleton.Find(".placeholder.ms-auto").Length() != 1 ||
			skeleton.ChildrenFiltered("div").Length() != 6 {
			t.Errorf("skeleton %d must include a name, five stars, a date and five text lines", i)
		}
	})
	if len(source.reviewCalls) != 0 {
		t.Fatal("page and rating requests must not fetch reviews")
	}
}

func TestDocumentDetailGoodReadsReviewsHeader(t *testing.T) {
	for _, tt := range []struct {
		name   string
		source *fakeBookSource
		slug   string
		want   int
	}{
		{name: "matched", source: &fakeBookSource{}, slug: "linked", want: 1},
		{name: "zero score", source: &fakeBookSource{book: biblioreads.BookResponse{Rating: "0.00"}}, slug: "linked", want: 1},
		{name: "unavailable", source: &fakeBookSource{err: biblioreads.ErrUnavailable}, slug: "linked", want: 1},
		{name: "disabled", slug: "linked"},
		{name: "unmatched", source: &fakeBookSource{}, slug: "unmatched"},
		{name: "not looked up", source: &fakeBookSource{}, slug: "unlinked"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, _ := newGoodReadsTestApp(t, tt.source)
			page, err := goquery.NewDocumentFromReader(strings.NewReader(getBody(t, app, "/documents/"+tt.slug)))
			if err != nil {
				t.Fatal(err)
			}
			header := page.Find(`#document-reviews-` + tt.slug + ` h2 a`)
			if header.Length() != tt.want {
				t.Fatalf("expected %d reviews headings, got %d", tt.want, header.Length())
			}
			if tt.want == 1 {
				url, _ := header.Attr("href")
				if header.Text() != "Goodreads user reviews" || url != "https://www.goodreads.com/book/show/12207444-la-reina-dragon" {
					t.Fatal("expected a linked Goodreads reviews heading")
				}
			}
			if tt.source != nil && (len(tt.source.ids) != 0 || len(tt.source.reviewCalls) != 0 || len(tt.source.queries) != 0) {
				t.Fatal("rendering the heading must not make any API calls")
			}
		})
	}
}

func TestDocumentDetailGoodReadsReviews(t *testing.T) {
	source := &fakeBookSource{
		book: biblioreads.BookResponse{Rating: "4.20", ResourceID: "work-id", LegacyBookID: 12207444},
		reviews: biblioreads.ReviewsResponse{Reviews: []biblioreads.Review{
			{Reviewer: biblioreads.Reviewer{Name: "<script>Reviewer</script>"}, Rating: 4,
				Text:      `<p>Great<br />book</p><script>alert(1)</script><img src=x onerror="alert(1)">`,
				CreatedAt: "Fri Oct 30 2026", ReviewURL: "/review/show/123"},
			{Reviewer: biblioreads.Reviewer{Name: "Spoiler reviewer"}, Text: "The ending", Spoiler: true},
		}},
	}
	app, idx := newGoodReadsTestApp(t, source)
	page, err := goquery.NewDocumentFromReader(strings.NewReader(getBody(t, app, "/documents/linked/reviews")))
	if err != nil {
		t.Fatal(err)
	}
	if len(source.reviewCalls) != 1 || source.reviewCalls[0] != (reviewCall{"work-id", 12207444, "", "en"}) {
		t.Fatalf("unexpected review calls: %+v", source.reviewCalls)
	}
	if page.Find("article").Length() != 2 || page.Find(`span[role="img"][aria-label="4/5"]`).Length() != 1 {
		t.Fatal("expected reviews and their ratings")
	}
	rating := page.Find(`span[role="img"][aria-label="4/5"]`)
	if rating.Find("i.bi-star-fill").Length() != 4 || rating.Find("i.bi-star").Length() != 1 || rating.Find(`i[aria-hidden="true"]`).Length() != 5 {
		t.Fatal("expected four solid stars and one hollow star")
	}
	page.Find("article").Each(func(i int, article *goquery.Selection) {
		content := article.Find(".review-content.clamp")
		button := article.Find(`button.review-toggle[aria-expanded="false"]`)
		id, _ := content.Attr("id")
		target, _ := button.Attr("aria-controls")
		if content.Length() != 1 || button.Length() != 1 || id == "" || target != id || strings.TrimSpace(button.Text()) != "Show more" {
			t.Errorf("review %d must start collapsed with an accessible expansion button", i)
		}
	})
	if page.Find("details .review-content.clamp").Length() != 1 {
		t.Fatal("spoiler content must also start with a collapsed preview")
	}
	if page.Find("script, img, [onerror]").Length() != 0 || page.Find("article p br").Length() != 1 {
		t.Fatal("expected sanitized review HTML retaining paragraph breaks")
	}
	if page.Find("article strong").First().Text() != "<script>Reviewer</script>" {
		t.Fatal("reviewer names must be escaped")
	}
	if page.Find("details:not([open])").Length() != 1 || !strings.Contains(page.Find("details").Text(), "The ending") {
		t.Fatal("spoiler review must be collapsed")
	}
	if page.Find(`a[href="https://www.goodreads.com/review/show/123"]`).Length() != 1 {
		t.Fatal("expected a link to the original review")
	}
	if page.Find(`a time.locale[datetime="2026-10-30"]`).Text() != "2026-10-30" {
		t.Fatal("expected review dates in ISO format for client-side localization")
	}
	stored, err := idx.Document("linked")
	if err != nil || stored.GoodReadsResourceID != "work-id" || stored.GoodReadsLegacyBookID != 12207444 {
		t.Fatalf("expected review identifiers to be stored: %+v, %v", stored, err)
	}
}

func TestDocumentReviewFractionalStars(t *testing.T) {
	for _, tt := range []struct {
		rating float64
		full   int
		empty  int
		width  int
	}{
		{rating: 0},
		{rating: 0.25, empty: 4, width: 25},
		{rating: 3.5, full: 3, empty: 1, width: 50},
		{rating: 4.25, full: 4, width: 25},
		{rating: 4.75, full: 4, width: 75},
		{rating: 4, full: 4, empty: 1},
		{rating: 5, full: 5},
	} {
		t.Run(fmt.Sprint(tt.rating), func(t *testing.T) {
			source := &fakeBookSource{
				book: biblioreads.BookResponse{Rating: "4", ResourceID: "work", LegacyBookID: 1},
				reviews: biblioreads.ReviewsResponse{Reviews: []biblioreads.Review{
					{Reviewer: biblioreads.Reviewer{Name: "Reader"}, Rating: tt.rating},
				}},
			}
			app, _ := newGoodReadsTestApp(t, source)
			page, err := goquery.NewDocumentFromReader(strings.NewReader(getBody(t, app, "/documents/linked/reviews")))
			if err != nil {
				t.Fatal(err)
			}
			stars := page.Find(`article span[role="img"]`)
			if tt.rating == 0 {
				if stars.Length() != 0 {
					t.Fatal("unrated reviews must not display stars")
				}
				if page.Find("article header span.text-muted").Text() != "Not scored" {
					t.Fatal("unrated reviews must display Not scored")
				}
				return
			}
			if page.Find("article header span.text-muted").Length() != 0 {
				t.Fatal("scored reviews must not display the unrated label")
			}
			if label, _ := stars.Attr("aria-label"); label != fmt.Sprintf("%g/5", tt.rating) {
				t.Fatalf("expected exact accessible rating, got %q", label)
			}
			if stars.ChildrenFiltered("i.bi-star-fill").Length() != tt.full ||
				stars.ChildrenFiltered("i.bi-star").Length() != tt.empty ||
				stars.Children().Length() != 5 {
				t.Fatalf("unexpected full/empty star counts: %s", stars.Text())
			}
			partial := stars.Find(".review-star-fill")
			if tt.width == 0 {
				if partial.Length() != 0 {
					t.Fatal("whole ratings must not include a partial star")
				}
			} else if style, _ := partial.Attr("style"); style != fmt.Sprintf("width: %d%%", tt.width) ||
				stars.Find(`.review-star[aria-hidden="true"]`).Length() != 1 {
				t.Fatalf("expected a %d%% decorative star fill, got %q", tt.width, style)
			}
		})
	}
}

func TestDocumentReviewNotScoredTranslations(t *testing.T) {
	source := &fakeBookSource{
		book: biblioreads.BookResponse{Rating: "4", ResourceID: "work", LegacyBookID: 1},
		reviews: biblioreads.ReviewsResponse{Reviews: []biblioreads.Review{
			{Reviewer: biblioreads.Reviewer{Name: "Reader"}, Text: "A review without a rating"},
		}},
	}
	app, _ := newGoodReadsTestApp(t, source)
	for _, tt := range []struct {
		lang string
		text string
	}{
		{"en", "Not scored"},
		{"de", "Nicht bewertet"},
		{"es", "Sin puntuar"},
		{"fr", "Non noté"},
		{"ru", "Не оценено"},
	} {
		t.Run(tt.lang, func(t *testing.T) {
			page, err := goquery.NewDocumentFromReader(strings.NewReader(getBody(t, app, "/documents/linked/reviews?l="+tt.lang)))
			if err != nil {
				t.Fatal(err)
			}
			if got := page.Find("article header span.text-muted").Text(); got != tt.text {
				t.Fatalf("expected %q, got %q", tt.text, got)
			}
		})
	}
}

func TestDocumentDetailGoodReadsReviewsAvailability(t *testing.T) {
	for _, tt := range []struct {
		name     string
		source   *fakeBookSource
		slug     string
		status   int
		calls    int
		text     string
		language string
	}{
		{name: "disabled", slug: "linked", status: http.StatusOK},
		{name: "unmatched", source: &fakeBookSource{}, slug: "unmatched", status: http.StatusOK},
		{name: "no score", source: &fakeBookSource{book: biblioreads.BookResponse{ResourceID: "work", LegacyBookID: 1}}, slug: "linked", status: http.StatusOK, text: "Goodreads user reviews"},
		{name: "book unavailable", source: &fakeBookSource{err: errors.New("unavailable")}, slug: "linked", status: http.StatusInternalServerError},
		{name: "missing identifiers", source: &fakeBookSource{book: biblioreads.BookResponse{Rating: "4"}}, slug: "linked", status: http.StatusInternalServerError},
		{name: "reviews unavailable", source: &fakeBookSource{book: biblioreads.BookResponse{Rating: "4", ResourceID: "work", LegacyBookID: 1}, reviewErr: errors.New("unavailable")}, slug: "linked", status: http.StatusInternalServerError, calls: 1},
		{name: "empty reviews", source: &fakeBookSource{book: biblioreads.BookResponse{Rating: "4", ResourceID: "work", LegacyBookID: 1}}, slug: "linked", status: http.StatusOK, calls: 1, text: "No user reviews available in the current language"},
		{name: "empty Spanish reviews", source: &fakeBookSource{book: biblioreads.BookResponse{Rating: "4", ResourceID: "work", LegacyBookID: 1}}, slug: "linked", status: http.StatusOK, calls: 1, language: "es", text: "No hay reseñas de usuarios disponibles en el idioma actual"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, _ := newGoodReadsTestApp(t, tt.source)
			req := mustGetRequest(t, "/documents/"+tt.slug+"/reviews")
			req.Header.Set("Accept-Language", "en")
			if tt.language != "" {
				req.Header.Set("Accept-Language", tt.language)
			}
			response, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}

			defer response.Body.Close()
			mustReturnStatus(response, tt.status, t)
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if tt.status == http.StatusOK && tt.text == "" && strings.TrimSpace(string(body)) != "" {
				t.Fatalf("expected no reviews content, got %q", body)
			}
			if tt.text != "" && !strings.Contains(string(body), tt.text) {
				t.Fatalf("expected %q in %q", tt.text, body)
			}
			if tt.source != nil && len(tt.source.reviewCalls) != tt.calls {
				t.Fatalf("expected %d review requests, got %+v", tt.calls, tt.source.reviewCalls)
			}
		})
	}
}

func TestDocumentDetailGoodReadsReviewsAfterSearch(t *testing.T) {
	source := &fakeBookSource{
		results: []biblioreads.SearchResult{{Title: "Unlinked", Author: "Jane Doe", BookURL: "/book/show/1-unlinked", Rating: "4"}},
		book:    biblioreads.BookResponse{Rating: "4", ResourceID: "work-id", LegacyBookID: 1},
	}
	app, _ := newGoodReadsTestApp(t, source)
	getBody(t, app, "/documents/unlinked/goodreads")
	if len(source.ids) != 0 || len(source.reviewCalls) != 0 {
		t.Fatal("search rating must not scrape the book or load reviews")
	}
	getBody(t, app, "/documents/unlinked/reviews")
	if len(source.ids) != 1 || source.ids[0] != "1-unlinked" || len(source.reviewCalls) != 1 {
		t.Fatal("reviews must resolve identifiers for a search-only match")
	}

	source.book = biblioreads.BookResponse{Rating: "4"}
	getBody(t, app, "/documents/unlinked/reviews")
	if len(source.reviewCalls) != 2 || source.reviewCalls[1] != (reviewCall{"work-id", 1, "", "en"}) {
		t.Fatalf("expected stored identifiers to survive incomplete book responses, got %+v", source.reviewCalls)
	}
}
