package webserver_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/blevesearch/bleve/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
	"github.com/svera/coreander/v5/internal/datasource/wikidata"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/metadata"
	"github.com/svera/coreander/v5/internal/webserver"
	authorcontroller "github.com/svera/coreander/v5/internal/webserver/controller/author"
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

func mustBiblioReadsData(t *testing.T, document index.Document) biblioreads.DocumentData {
	t.Helper()
	data, err := index.DataSourceData[biblioreads.DocumentData](document, biblioreads.Source)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func withBiblioReadsData(t *testing.T, document index.Document, data biblioreads.DocumentData) index.Document {
	t.Helper()
	if err := index.SetDataSourceData(&document, biblioreads.Source, data); err != nil {
		t.Fatal(err)
	}
	return document
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

func newBiblioReadsTestApp(t *testing.T, source *fakeBookSource) (*fiber.App, *index.BleveIndexer) {
	t.Helper()
	return newDataSourceProvidersTestApp(t, source, nil, nil)
}

func TestReviewEnrichmentProgressBanner(t *testing.T) {
	source := &fakeBookSource{searchErr: biblioreads.ErrUnavailable}
	app, idx := newBiblioReadsTestApp(t, source)
	idx.BeginIndexing()
	page := getPage(t, app, "/documents/linked")
	banner := page.Find("#background-progress")
	if banner.Find(`[data-progress-kind="documents"]`).Length() != 1 ||
		banner.Find(`[data-progress-kind="data-sources"]`).Length() != 1 {
		t.Fatal("indexing and review progress must share a banner")
	}
	if banner.AttrOr("hx-trigger", "") != "every 5s" ||
		!strings.Contains(banner.Text(), "Score filters may omit documents") {
		t.Fatal("expected live polling and score-filter warning")
	}
	checkPending := func() {
		t.Helper()
		page := getPage(t, app, "/background-progress?l=en")
		reviews := page.Find(`[data-progress-kind="data-sources"]`)
		if !strings.Contains(reviews.Text(), "(Goodreads)") ||
			reviews.Find(`[role="progressbar"]`).AttrOr("aria-valuenow", "") != "66" {
			t.Fatalf("incorrect persisted lookup progress: %s", reviews.Text())
		}
		bar := reviews.Find(".progress-bar")
		if !bar.HasClass("bg-info-subtle") || !bar.HasClass("text-body-emphasis") || bar.HasClass("bg-info") {
			t.Fatal("review progress must use a theme-aware background with contrasting text")
		}
	}
	checkPending()
	for lang, label := range map[string]string{
		"en": "Retrieving ratings",
		"es": "Recuperando valoraciones",
		"de": "Bewertungen werden abgerufen",
		"fr": "Récupération des évaluations",
		"ru": "Получение оценок",
	} {
		localized := getPage(t, app, "/background-progress?l="+lang)
		reviews := localized.Find(`[data-progress-kind="data-sources"]`)
		if reviews.Find(`[role="progressbar"]`).AttrOr("aria-label", "") != label ||
			!strings.Contains(reviews.Find("p").First().Text(), label) {
			t.Errorf("%s: review progress must use the localized retrieval label %q", lang, label)
		}
	}
	spanish := getPage(t, app, "/background-progress?l=es")
	if !strings.Contains(spanish.Find("#background-progress").Text(), "La información de valoraciones está incompleta") {
		t.Fatal("live progress must retain the selected language")
	}
	if err := idx.EnrichDocuments(1); err == nil {
		t.Fatal("expected unavailable source to fail")
	}
	for lang, label := range map[string]string{
		"en": "Some ratings remain unresolved",
		"es": "Algunas valoraciones siguen sin resolverse",
		"de": "Einige Bewertungen bleiben ungeklärt",
		"fr": "Certaines évaluations restent non résolues",
		"ru": "Некоторые оценки остаются неполученными",
	} {
		localized := getPage(t, app, "/background-progress?l="+lang)
		reviews := localized.Find(`[data-progress-kind="data-sources"]`)
		if !strings.Contains(reviews.Find("p").First().Text(), label) {
			t.Errorf("%s: failed lookups must show incomplete results", lang)
		}
		if lang == "en" && !strings.Contains(reviews.Text(), "Restart Coreander to retry pending lookups.") {
			t.Fatal("failed progress must explain how to retry")
		}
		if reviews.Find(`[role="progressbar"]`).Length() != 0 {
			t.Fatal("finished retrieval must not leave a stuck progress bar")
		}
	}
	source.searchErr = nil
	if err := idx.EnrichDocuments(1); err != nil {
		t.Fatal(err)
	}
	page = getPage(t, app, "/background-progress?l=en")
	if page.Find(`[data-progress-kind="data-sources"]`).Length() != 0 ||
		page.Find(`[data-progress-kind="documents"]`).Length() != 1 {
		t.Fatal("completion must remove only the review bar, including no-match lookups")
	}
	response, err := app.Test(mustGetRequest(t, "/background-progress"))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("live progress responses must not be cached")
	}
}

func newDataSourceProvidersTestApp(t *testing.T, source *fakeBookSource, provider document.ReviewProvider, dataSourceLinkProvider authorcontroller.DataSourceLinkProvider) (*fiber.App, *index.BleveIndexer) {
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
	var adapter *biblioreads.Enricher
	if source != nil {
		adapter = biblioreads.NewEnricher(source)
	} else {
		adapter = biblioreads.NewEnricher(nil)
	}
	if source != nil {
		idxCfg.DocumentEnrichers = []index.DocumentEnricher{adapter}
	}
	idx := index.NewBleve(documents, authors, appFS, testLibraryDir, nil, idxCfg)
	adapter.SetIndex(idx)
	if source != nil {
		provider = biblioreads.NewReviewProvider(source, adapter)
	}
	biblioReadsLinks := biblioreads.NewReviewProvider(nil, adapter)
	t.Cleanup(func() { _ = idx.Close() })

	for _, doc := range []index.Document{withBiblioReadsData(t, index.Document{
		ID: "linked.epub", Slug: "linked",
		Metadata: metadata.Metadata{Title: "La Reina Dragon", Format: "EPUB"},

		DataSourceScoreValue: 3.82,
	}, biblioreads.DocumentData{URL: "/book/show/12207444-la-reina-dragon", Enriched: true}), {
		ID: "unlinked.epub", Slug: "unlinked",
		Metadata:     metadata.Metadata{Title: "Unlinked", Authors: []string{"Jane Doe", "John Doe"}, Format: "EPUB"},
		AuthorsSlugs: []string{"jane-doe", "john-doe"},
	}, withBiblioReadsData(t, index.Document{
		ID: "unmatched.epub", Slug: "unmatched",
		Metadata: metadata.Metadata{Title: "Unmatched", Format: "EPUB"},
	}, biblioreads.DocumentData{Enriched: true}),
	} {
		if err := index.IndexDocument(documents, doc.ID, doc); err != nil {
			t.Fatal(err)
		}
	}

	cfg := defaultTestConfig()
	cfg.ReviewSourceConfigured = provider != nil
	sender := &infrastructure.NoEmail{}
	controllers := webserver.SetupControllers(cfg, db, idx, sender, appFS, wikidata.NewWikidataSource(wikidata.Gowikidata{}), provider, wikidata.NewWikidataSource(wikidata.Gowikidata{}), biblioReadsLinks, dataSourceLinkProvider)
	app := webserver.New(cfg, controllers, sender, idx, &model.UserRepository{DB: db})
	t.Cleanup(func() { _ = app.Shutdown() })
	return app, idx
}

func TestDocumentDetailBiblioReadsRating(t *testing.T) {
	source := &fakeBookSource{book: biblioreads.BookResponse{
		Rating: "3.82", RatingCount: "2,285",
		ResourceID: "work-id", LegacyBookID: 12207444,
	}}
	app, idx := newBiblioReadsTestApp(t, source)

	placeholder := documentScorePlaceholders(t, getBody(t, app, "/documents/linked"))
	if placeholder.Length() != 1 {
		t.Fatalf("expected one BiblioReads placeholder, got %d", placeholder.Length())
	}
	if url, _ := placeholder.Attr("hx-get"); url != "/documents/linked/score" {
		t.Errorf("unexpected placeholder URL %q", url)
	}
	if placeholder.ParentsFiltered(`[id="document-metadata-linked"]`).Length() != 1 {
		t.Error("expected the placeholder inside the document metadata list")
	}

	page := getPage(t, app, "/documents/linked/score")
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
	if mustBiblioReadsData(t, stored).ResourceID != source.book.ResourceID || mustBiblioReadsData(t, stored).LegacyBookID != source.book.LegacyBookID {
		t.Fatalf("expected book identifiers to be stored, got %+v", stored)
	}
}

func TestBiblioReadsRatingCountEscaping(t *testing.T) {
	for _, count := range []string{
		"2,285",
		`<img src=x onerror=alert(1)><script>alert(1)</script>`,
		`&lt;img src=x&gt; & "quoted" 'count'`,
	} {
		t.Run(count, func(t *testing.T) {
			source := &fakeBookSource{book: biblioreads.BookResponse{
				Rating: "3.82", RatingCount: count,
			}}
			app, idx := newBiblioReadsTestApp(t, source)
			for _, language := range []struct {
				code  string
				label string
			}{
				{code: "en", label: "ratings"},
				{code: "de", label: "Bewertungen"},
			} {
				for _, path := range []string{
					"/documents/linked/score?l=" + language.code,
					"/search?type=documents&search=La&l=" + language.code,
				} {
					page := getPage(t, app, path)
					row := page.Find("i.bi-star-fill").Closest("li")
					if row.Length() != 1 {
						t.Fatalf("%s: expected one rating row, got %d", path, row.Length())
					}
					if row.Find("small").Text() != "("+count+" "+language.label+")" {
						t.Errorf("%s: rating count must remain literal text, got %q", path, row.Find("small").Text())
					}
					if row.Find("img, script, [onerror]").Length() != 0 {
						t.Errorf("%s: provider rating count must not create HTML elements", path)
					}
				}
			}
			stored, err := idx.Document("linked")
			if err != nil {
				t.Fatal(err)
			}
			if got := mustBiblioReadsData(t, stored).RatingCount; got != count {
				t.Fatalf("escaping must not change the stored count: got %q", got)
			}
		})
	}
}

func TestDocumentUpdateBiblioReads(t *testing.T) {
	source := &fakeBookSource{book: biblioreads.BookResponse{
		Rating: "3.82", ResourceID: "old-work", LegacyBookID: 12207444,
	}}
	app, idx := newBiblioReadsTestApp(t, source)
	getBody(t, app, "/documents/linked/score")
	adminCookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatal(err)
	}
	response, err := putRequest(url.Values{
		"bookURL": {"https://www.goodreads.com/book/show/1368314.El_Cercano_Oriente?from_search=true"},
	}, adminCookie, app, "/documents/linked/review-link", t)
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
	if mustBiblioReadsData(t, stored).URL != "/book/show/1368314.El_Cercano_Oriente" ||
		stored.DataSourceScoreValue != 0 || mustBiblioReadsData(t, stored).ResourceID != "" || mustBiblioReadsData(t, stored).LegacyBookID != 0 ||
		!mustBiblioReadsData(t, stored).Enriched {
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

func TestDocumentUpdateBiblioReadsValidation(t *testing.T) {
	app, idx := newBiblioReadsTestApp(t, &fakeBookSource{})
	adminCookie, err := login(app, "admin@example.com", "admin", t)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", "https://example.org/book/show/123", "https://www.goodreads.com/author/show/16667"} {
		response, err := putRequest(url.Values{"bookURL": {value}}, adminCookie, app, "/documents/linked/review-link", t)
		if err != nil {
			t.Fatal(err)
		}
		mustReturnStatus(response, http.StatusUnprocessableEntity, t)
		page, err := goquery.NewDocumentFromReader(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if page.Find(`form#review-link-form-linked input[aria-invalid="true"]`).Length() != 1 ||
			page.Find(`[role="alert"]`).Text() != "Enter a valid Goodreads book URL." {
			t.Fatal("invalid URLs must render inline validation feedback")
		}
		if got, _ := page.Find(`input[name="bookURL"]`).Attr("value"); got != value {
			t.Fatalf("invalid input should be retained for correction, got %q", got)
		}
		stored, err := idx.Document("linked")
		if err != nil || mustBiblioReadsData(t, stored).URL != "/book/show/12207444-la-reina-dragon" || stored.DataSourceScoreValue != 3.82 {
			t.Fatalf("invalid input must not change the stored match: %+v, %v", stored, err)
		}
	}
	response, err := putRequest(url.Values{
		"bookURL": {"https://www.goodreads.com/book/show/1368314"},
	}, adminCookie, app, "/documents/missing/review-link", t)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	mustReturnStatus(response, http.StatusNotFound, t)
}

func TestDocumentDetailUpdatesStoredBiblioReadsRating(t *testing.T) {
	source := &fakeBookSource{book: biblioreads.BookResponse{
		Rating: "4.12", ResourceID: "work-id", LegacyBookID: 12207444,
	}}
	app, idx := newBiblioReadsTestApp(t, source)

	page := getPage(t, app, "/documents/linked/score")
	link := page.Find(`li a[href="https://www.goodreads.com/book/show/12207444-la-reina-dragon"]`)
	if got := strings.TrimSpace(link.Text()); got != "4.12" {
		t.Fatalf("expected latest Goodreads rating 4.12, got %q", got)
	}
	stored, err := idx.Document("linked")
	if err != nil {
		t.Fatal(err)
	}
	if stored.DataSourceScoreValue != 4.12 {
		t.Fatalf("expected latest rating and searchable value to be stored, got %+v", stored)
	}
	results, err := idx.Search(index.SearchFields{DataSourceScoreFrom: 4.1, DataSourceScoreTo: new(4.2)}, 1, 10)
	if err != nil || results.TotalHits() != 1 {
		t.Fatalf("expected document to match updated rating range, got %d results and error %v", results.TotalHits(), err)
	}
}

func TestDocumentDetailKeepsStoredBiblioReadsRatingWhenScraperReturnsZero(t *testing.T) {
	source := &fakeBookSource{book: biblioreads.BookResponse{
		Rating: "0.00", RatingCount: "650", ResourceID: "work-id", LegacyBookID: 12207444,
	}}
	app, idx := newBiblioReadsTestApp(t, source)

	page := getPage(t, app, "/documents/linked/score")
	link := page.Find(`li a[href="https://www.goodreads.com/book/show/12207444-la-reina-dragon"]`)
	if got := strings.TrimSpace(link.Text()); got != "3.82" {
		t.Fatalf("expected known Goodreads rating to be retained, got %q", got)
	}
	stored, err := idx.Document("linked")
	if err != nil {
		t.Fatal(err)
	}
	if stored.DataSourceScoreValue != 3.82 {
		t.Fatalf("zero scraper rating must not replace indexed Goodreads score, got %+v", stored)
	}
}

func TestDocumentDetailBiblioReadsSearchesUnlinkedDocuments(t *testing.T) {
	source := &fakeBookSource{
		book:    biblioreads.BookResponse{Rating: "4.20", RatingCount: "10", ResourceID: "work-id", LegacyBookID: 1},
		results: []biblioreads.SearchResult{{Title: "Unlinked", Author: "Jane Doe", BookURL: "/book/show/1-unlinked", AuthorURL: "/author/show/1.Jane_Doe", Rating: "4.10"}, {BookURL: "/book/show/2-other", Rating: "5.00"}},
	}
	app, idx := newBiblioReadsTestApp(t, source)

	if got := documentScorePlaceholders(t, getBody(t, app, "/documents/unlinked")).Length(); got != 1 {
		t.Fatalf("expected a placeholder for a document not looked up yet, got %d", got)
	}

	page := getPage(t, app, "/documents/unlinked/score")
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
	if mustBiblioReadsData(t, stored).URL != "/book/show/1-unlinked" || !mustBiblioReadsData(t, stored).Enriched {
		t.Errorf("expected the found link to be stored, got %+v", stored)
	}
	author, err := idx.Author("jane-doe", "")
	if err != nil || author.DataSourceLinks["goodreads"] != "/author/show/1.Jane_Doe" || author.DocumentCount != 1 {
		t.Fatalf("expected missing author entry created with URL and document count, got %+v, %v", author, err)
	}
	if mustBiblioReadsData(t, stored).ResourceID != "" || mustBiblioReadsData(t, stored).LegacyBookID != 0 {
		t.Fatalf("search-only lookup must not populate scraper identifiers, got %+v", stored)
	}

	if err := idx.EnrichDocuments(10); err != nil {
		t.Fatal(err)
	}
	page = getPage(t, app, "/documents/unlinked/score")
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

func TestDocumentDetailBiblioReadsSkipsUnmatchedDocuments(t *testing.T) {
	source := &fakeBookSource{book: biblioreads.BookResponse{Rating: "3.82"}}
	app, _ := newBiblioReadsTestApp(t, source)
	if got := documentScorePlaceholders(t, getBody(t, app, "/documents/unmatched")).Length(); got != 1 {
		t.Errorf("expected an async score placeholder, got %d", got)
	}
	for _, path := range []string{"/documents/unmatched/score"} {
		page := getPage(t, app, path)
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

func TestDocumentDetailBiblioReadsIdentifiersWithoutRating(t *testing.T) {
	source := &fakeBookSource{book: biblioreads.BookResponse{ResourceID: "work-id", LegacyBookID: 12207444}}
	app, idx := newBiblioReadsTestApp(t, source)
	page := getPage(t, app, "/documents/linked/score")
	if strings.TrimSpace(page.Find("li").Text()) != "Not scored" || page.Find("h2").Text() != "Goodreads user reviews" {
		t.Fatal("expected no score and a reviews heading for a matched book without a rating")
	}
	stored, err := idx.Document("linked")
	if err != nil {
		t.Fatal(err)
	}
	if mustBiblioReadsData(t, stored).ResourceID != "work-id" || mustBiblioReadsData(t, stored).LegacyBookID != 12207444 {
		t.Fatalf("expected identifiers even without a rating, got %+v", stored)
	}

	for _, book := range []biblioreads.BookResponse{
		{},
		{ResourceID: "partial-work"},
		{LegacyBookID: 2},
	} {
		source.book = book
		getBody(t, app, "/documents/linked/score")
		stored, err = idx.Document("linked")
		if err != nil {
			t.Fatal(err)
		}
		if mustBiblioReadsData(t, stored).ResourceID != "work-id" || mustBiblioReadsData(t, stored).LegacyBookID != 12207444 {
			t.Fatalf("incomplete response overwrote stored identifiers: %+v", stored)
		}
	}
}

func TestDocumentDetailBiblioReadsSearchDoesNotScrape(t *testing.T) {
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
			app, idx := newBiblioReadsTestApp(t, source)
			body := strings.TrimSpace(getBody(t, app, "/documents/unlinked/score"))
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
			if err != nil || mustBiblioReadsData(t, stored).Enriched != tt.enriched {
				t.Fatalf("unexpected stored lookup status: %+v, %v", stored, err)
			}
		})
	}
}

func TestDocumentDetailBiblioReadsReviews(t *testing.T) {
	source := &fakeBookSource{
		book: biblioreads.BookResponse{Rating: "4.20", ResourceID: "work-id", LegacyBookID: 12207444},
		reviews: biblioreads.ReviewsResponse{Reviews: []biblioreads.Review{
			{Reviewer: biblioreads.Reviewer{Name: "<script>Reviewer</script>"}, Rating: 4,
				Text:      `<p>Great<br />book</p><script>alert(1)</script><img src=x onerror="alert(1)">`,
				CreatedAt: "Fri Oct 30 2026", ReviewURL: "/review/show/123"},
			{Reviewer: biblioreads.Reviewer{Name: "Spoiler reviewer"}, Text: "The ending", Spoiler: true},
		}},
	}
	app, idx := newBiblioReadsTestApp(t, source)
	page := getPage(t, app, "/documents/linked/reviews")
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
	if err != nil || mustBiblioReadsData(t, stored).ResourceID != "work-id" || mustBiblioReadsData(t, stored).LegacyBookID != 12207444 {
		t.Fatalf("expected review identifiers to be stored: %+v, %v", stored, err)
	}
}

func TestDocumentDetailBiblioReadsReviewsAvailability(t *testing.T) {
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
			app, _ := newBiblioReadsTestApp(t, tt.source)
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

func TestDocumentDetailBiblioReadsReviewsAfterSearch(t *testing.T) {
	source := &fakeBookSource{
		results: []biblioreads.SearchResult{{Title: "Unlinked", Author: "Jane Doe", BookURL: "/book/show/1-unlinked", Rating: "4"}},
		book:    biblioreads.BookResponse{Rating: "4", ResourceID: "work-id", LegacyBookID: 1},
	}
	app, _ := newBiblioReadsTestApp(t, source)
	getBody(t, app, "/documents/unlinked/score")
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
