package webserver_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/reviews"
	"github.com/svera/coreander/v5/internal/webserver"
	"github.com/svera/coreander/v5/internal/webserver/controller/document"
)

func TestDocumentReviewsCanSelectSupportedLanguage(t *testing.T) {
	provider := &fakeReviewProvider{
		score: reviews.Score{Source: "Another service"},
		page:  reviews.Page{Source: "Another service", Available: true},
	}
	app, _ := newReviewProviderTestApp(t, provider)
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
	if len(provider.reviewLanguages) != 1 || provider.reviewLanguages[0] != "fr" {
		t.Fatalf("expected reviews in French, got %+v", provider.reviewLanguages)
	}
	if page.Find("h2").Text() != "Reseñas de usuarios de Another service" {
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
	provider := &fakeReviewProvider{
		score: reviews.Score{Source: "Another service"},
		page:  reviews.Page{Source: "Another service", Available: true},
	}
	app, _ := newReviewProviderTestApp(t, provider)
	response, err := app.Test(mustGetRequest(t, "/documents/linked/reviews?language=zz"))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	mustReturnStatus(response, http.StatusBadRequest, t)
	if len(provider.reviewLanguages) != 0 {
		t.Fatalf("unsupported language must not be sent to the provider, got %+v", provider.reviewLanguages)
	}
}

type fakeReviewProvider struct {
	score               reviews.Score
	page                reviews.Page
	scoreErr            error
	reviewsErr          error
	canEditDocumentLink bool
	scoreSlugs          []string
	reviewSlugs         []string
	reviewLanguages     []string
}

func (f *fakeReviewProvider) CanEditDocumentLink() bool {
	return f.canEditDocumentLink
}

func (f *fakeReviewProvider) Name() string {
	return f.score.Source
}

func (f *fakeReviewProvider) UpdateDocumentLink(index.Document, string) error {
	return nil
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
			provider := &fakeReviewProvider{
				score: reviews.Score{Source: "Another service"},
				page:  reviews.Page{Source: "Another service", Available: true},
			}
			app, _ := newReviewProviderTestApp(t, provider)
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
			if len(provider.reviewLanguages) != 1 || provider.reviewLanguages[0] != tt.want {
				t.Fatalf("expected review language %q, got %+v", tt.want, provider.reviewLanguages)
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
	app, _ := newReviewProviderTestApp(t, provider)
	searchPage, err := goquery.NewDocumentFromReader(strings.NewReader(getBody(t, app, "/search?type=documents&search=La")))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(searchPage.Find(`fieldset legend`).Text(), "Another service score") ||
		searchPage.Find(`#sort-by option[value="review-score-asc"]`).Text() != "Another service score (ascending)" {
		t.Fatal("search filters and sort options must use the active review provider's name")
	}
	// This document has a cached Goodreads non-match; it must still use the chosen provider.
	detail := getBody(t, app, "/documents/unmatched")
	if documentScorePlaceholders(t, detail).Length() != 1 || len(provider.scoreSlugs) != 0 || len(provider.reviewSlugs) != 0 {
		t.Fatal("detail must defer lookups to the selected provider")
	}
	for _, path := range []string{"/documents/unmatched/score"} {
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
	if len(provider.scoreSlugs) != 1 || len(provider.reviewSlugs) != 1 || provider.reviewSlugs[0] != "unmatched" {
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
					Source: "Another service", Value: "0.00", Count: tt.count,
					URL: "https://example.org/books/book-1",
				},
			}
			app, _ := newReviewProviderTestApp(t, provider)
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

func newReviewProviderTestApp(t *testing.T, provider document.ReviewProvider) (*fiber.App, *index.BleveIndexer) {
	t.Helper()
	return newDataSourceProvidersTestApp(t, nil, provider, nil)
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

func documentScorePlaceholders(t *testing.T, html string) *goquery.Selection {
	t.Helper()
	page, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	return page.Find(`li[hx-get$="/score"]`)
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
			provider := &fakeReviewProvider{
				score: reviews.Score{Source: "Another service"},
				page: reviews.Page{Source: "Another service", Available: true, Reviews: []reviews.Review{
					{Reviewer: "Reader", Rating: tt.rating},
				}},
			}
			app, _ := newReviewProviderTestApp(t, provider)
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
	provider := &fakeReviewProvider{
		score: reviews.Score{Source: "Another service"},
		page: reviews.Page{Source: "Another service", Available: true, Reviews: []reviews.Review{
			{Reviewer: "Reader", Text: "A review without a rating"},
		}},
	}
	app, _ := newReviewProviderTestApp(t, provider)
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
