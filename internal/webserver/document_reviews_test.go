package webserver_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v3"
	datasourcemodel "github.com/svera/coreander/v5/internal/datasource/model"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/webserver"
	"github.com/svera/coreander/v5/internal/webserver/controller/document"
)

func TestDocumentReviewsCanSelectSupportedLanguage(t *testing.T) {
	provider := &fakeReviewProvider{
		score: datasourcemodel.Score{Source: "Another service"},
		page:  datasourcemodel.ReviewPage{Source: "Another service", Available: true},
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
		score: datasourcemodel.Score{Source: "Another service"},
		page:  datasourcemodel.ReviewPage{Source: "Another service", Available: true},
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
	score               datasourcemodel.Score
	page                datasourcemodel.ReviewPage
	scoreErr            error
	reviewsErr          error
	canEditDocumentLink bool
	scoreSlugs          []string
	reviewSlugs         []string
	reviewLanguages     []string
	documentURL         string
}

func (f *fakeReviewProvider) CanEditDocumentLink() bool {
	return f.canEditDocumentLink
}

func (f *fakeReviewProvider) Name() string {
	return f.score.Source
}

func (f *fakeReviewProvider) DocumentLink(index.Document) datasourcemodel.DataSourceLink {
	return datasourcemodel.DataSourceLink{Source: f.Name(), URL: f.documentURL}
}

func (f *fakeReviewProvider) UpdateDocumentLink(index.Document, string) error {
	return nil
}

func (f *fakeReviewProvider) Score(_ context.Context, document index.Document) (datasourcemodel.Score, error) {
	f.scoreSlugs = append(f.scoreSlugs, document.Slug)
	return f.score, f.scoreErr
}

func (f *fakeReviewProvider) StoredScore(index.Document) (datasourcemodel.Score, error) {
	return f.score, f.scoreErr
}

func (f *fakeReviewProvider) Reviews(_ context.Context, document index.Document, language string) (datasourcemodel.ReviewPage, error) {
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
				score: datasourcemodel.Score{Source: "Another service"},
				page:  datasourcemodel.ReviewPage{Source: "Another service", Available: true},
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
		score: datasourcemodel.Score{Source: "Another service", Value: "4.5", Count: "123", URL: "https://example.org/books/book-1"},
		page: datasourcemodel.ReviewPage{
			Source: "Another service", URL: "https://example.org/books/book-1", Available: true,
			Reviews: []datasourcemodel.Review{{
				Reviewer: "Reader", Rating: 5, Text: "<p>Excellent</p><script>unsafe()</script>",
				CreatedAt: time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC), URL: "https://example.org/reviews/review-1",
			}},
		},
	}
	app, _ := newReviewProviderTestApp(t, provider)
	searchPage := getPage(t, app, "/search?type=documents&search=La")
	if !strings.Contains(searchPage.Find(`fieldset legend`).Text(), "Another service score") ||
		searchPage.Find(`#sort-by option[value="review-score-asc"]`).Text() != "Another service score (ascending)" {
		t.Fatal("search filters and sort options must use the active review provider's name")
	}
	if searchPage.Find(`#list a[href="https://example.org/books/book-1"]`).Length() != 1 ||
		searchPage.Find(`#list a[href^="https://www.goodreads.com/book/"]`).Length() != 0 {
		t.Fatal("cached search-result scores must use the configured provider")
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
				score: datasourcemodel.Score{
					Source: "Another service", Value: "0.00", Count: tt.count,
					URL: "https://example.org/books/book-1",
				},
			}
			app, _ := newReviewProviderTestApp(t, provider)
			page := getPage(t, app, "/documents/unmatched/score")
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

func getPage(t *testing.T, app *fiber.App, path string) *goquery.Document {
	t.Helper()
	page, err := goquery.NewDocumentFromReader(strings.NewReader(getBody(t, app, path)))
	if err != nil {
		t.Fatal(err)
	}
	return page
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
				score: datasourcemodel.Score{Source: "Another service"},
				page: datasourcemodel.ReviewPage{Source: "Another service", Available: true, Reviews: []datasourcemodel.Review{
					{Reviewer: "Reader", Rating: tt.rating},
				}},
			}
			app, _ := newReviewProviderTestApp(t, provider)
			page := getPage(t, app, "/documents/linked/reviews")
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
		score: datasourcemodel.Score{Source: "Another service"},
		page: datasourcemodel.ReviewPage{Source: "Another service", Available: true, Reviews: []datasourcemodel.Review{
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
			page := getPage(t, app, "/documents/linked/reviews?l="+tt.lang)
			if got := page.Find("article header span.text-muted").Text(); got != tt.text {
				t.Fatalf("expected %q, got %q", tt.text, got)
			}
		})
	}
}

func TestDocumentScoreNoScoreTranslations(t *testing.T) {
	provider := &fakeReviewProvider{score: datasourcemodel.Score{Source: "Another service"}}
	app, _ := newReviewProviderTestApp(t, provider)
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
		for _, path := range []string{"/documents/unmatched/score"} {
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
	if len(provider.reviewSlugs) != 0 {
		t.Fatal("score requests must not fetch reviews")
	}
}

func TestDocumentReviewsLazyPlaceholder(t *testing.T) {
	provider := &fakeReviewProvider{
		score:       datasourcemodel.Score{Source: "Another service", Value: "3.82", URL: "https://example.org/books/book-1"},
		documentURL: "https://example.org/books/book-1",
	}
	app, _ := newReviewProviderTestApp(t, provider)
	page := getPage(t, app, "/documents/linked")
	target := page.Find("#document-reviews-linked")
	if target.Length() != 1 || target.Is("[hx-get]") {
		t.Fatal("expected an inactive reviews target before the rating loads")
	}
	if target.Find("h2").Text() != "Another service user reviews" {
		t.Fatal("expected the reviews heading before the score loads")
	}
	if !target.Is(".document-detail-content > :last-child") {
		t.Fatal("reviews must appear at the bottom of the document content")
	}
	page = getPage(t, app, "/documents/linked/score")
	placeholder := page.Find(`#document-reviews-linked[hx-get="/documents/linked/reviews"][hx-trigger="intersect once"][hx-swap-oob="outerHTML"]`)
	if placeholder.Length() != 1 {
		t.Fatal("expected successful rating to enable reviews on intersection only")
	}
	if placeholder.Find("h2").Text() != "Another service user reviews" || placeholder.Find(`[aria-hidden="true"] h2`).Length() != 0 {
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
	if len(provider.reviewSlugs) != 0 {
		t.Fatal("page and rating requests must not fetch reviews")
	}
}

func TestDocumentReviewsHeader(t *testing.T) {
	for _, tt := range []struct {
		name     string
		provider *fakeReviewProvider
		slug     string
		want     int
	}{
		{name: "matched", provider: &fakeReviewProvider{documentURL: "https://example.org/books/book-1"}, slug: "linked", want: 1},
		{name: "zero score", provider: &fakeReviewProvider{score: datasourcemodel.Score{Value: "0.00"}, documentURL: "https://example.org/books/book-1"}, slug: "linked", want: 1},
		{name: "unavailable", provider: &fakeReviewProvider{scoreErr: errors.New("unavailable"), documentURL: "https://example.org/books/book-1"}, slug: "linked", want: 1},
		{name: "disabled", slug: "linked"},
		{name: "unmatched", provider: &fakeReviewProvider{}, slug: "unmatched"},
		{name: "not looked up", provider: &fakeReviewProvider{}, slug: "unlinked"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var provider document.ReviewProvider
			if tt.provider != nil {
				tt.provider.score.Source = "Another service"
				provider = tt.provider
			}
			app, _ := newReviewProviderTestApp(t, provider)
			page := getPage(t, app, "/documents/"+tt.slug)
			header := page.Find(`#document-reviews-` + tt.slug + ` h2 a`)
			if header.Length() != tt.want {
				t.Fatalf("expected %d reviews headings, got %d", tt.want, header.Length())
			}
			if tt.want == 1 {
				url, _ := header.Attr("href")
				if header.Text() != "Another service user reviews" || url != "https://example.org/books/book-1" {
					t.Fatal("expected a linked provider reviews heading")
				}
			}
			if tt.provider != nil && (len(tt.provider.scoreSlugs) != 0 || len(tt.provider.reviewSlugs) != 0) {
				t.Fatal("rendering the heading must not make any API calls")
			}
		})
	}
}

func TestDocumentReviewsDisabled(t *testing.T) {
	app, _ := newReviewProviderTestApp(t, nil)
	for _, slug := range []string{"linked", "unlinked", "unmatched"} {
		body := getBody(t, app, "/documents/"+slug)
		if got := documentScorePlaceholders(t, body).Length(); got != 0 {
			t.Errorf("%s: expected no placeholder when reviews are disabled, got %d", slug, got)
		}
		if body := strings.TrimSpace(getBody(t, app, "/documents/"+slug+"/score")); body != "" {
			t.Errorf("%s: expected empty response, got %q", slug, body)
		}
		if strings.Contains(body, "bi-star-fill") {
			t.Errorf("%s: expected no score row when integration is disabled", slug)
		}
		if strings.Contains(body, `id="document-reviews-`) {
			t.Errorf("%s: expected no reviews target when integration is disabled", slug)
		}
	}
}

func TestDocumentScoreRendersNothingWhenUnavailable(t *testing.T) {
	provider := &fakeReviewProvider{scoreErr: errors.New("unavailable")}
	app, _ := newReviewProviderTestApp(t, provider)
	if body := strings.TrimSpace(getBody(t, app, "/documents/linked/score")); body != "" {
		t.Errorf("expected empty response, got %q", body)
	}
}

func TestDocumentReviewLinkEditor(t *testing.T) {
	provider := &fakeReviewProvider{
		score:               datasourcemodel.Score{Source: "Another service", Value: "3.82", URL: "https://example.org/books/book-1"},
		documentURL:         "https://example.org/books/book-1",
		canEditDocumentLink: true,
	}
	app, _ := newReviewProviderTestApp(t, provider)
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
			control := page.Find(`li a[data-bs-toggle="collapse"][href="#edit-review-link-linked"]`)
			if control.Length() != tt.want {
				t.Fatalf("expected %d edit links, got %d", tt.want, control.Length())
			}
			if tt.want == 0 {
				if page.Find(`input[name="bookURL"]`).Length() != 0 {
					t.Fatal("anonymous readers must not receive the editor")
				}
				return
			}
			if control.Text() != "" ||
				control.Find(`i.bi-pencil[alt="Edit Another service link"]`).Length() != 1 ||
				control.Filter(`[title="Edit Another service link"][aria-label="Edit Another service link"]`).Length() != 1 ||
				page.Find(`#edit-review-link-linked.collapse:not(.show)`).Length() != 1 ||
				page.Find(`form[hx-put="/documents/linked/review-link"][hx-target="this"]`).Length() != 1 ||
				page.Find(`label[for="document-book-url-linked"]`).Length() != 1 {
				t.Fatal("expected accessible, initially collapsed Another service editor beside the score")
			}
			if value, _ := page.Find(`input[name="bookURL"]`).Attr("value"); value != "https://example.org/books/book-1" {
				t.Fatalf("editor must be prefilled with the current link, got %q", value)
			}
		})
	}

	provider.scoreErr = errors.New("unavailable")
	response, err := getRequest(adminCookie, app, "/documents/linked/score", t)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	page, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page.Find("li").Text(), "Unable to load Another service score") ||
		page.Find(`input[name="bookURL"]`).Length() != 1 {
		t.Fatal("an unavailable score must not prevent administrators from correcting the link")
	}
}
