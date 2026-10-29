package webserver_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/blevesearch/bleve/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/metadata"
	"github.com/svera/coreander/v5/internal/webserver"
	"github.com/svera/coreander/v5/internal/webserver/controller/document"
	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
	"github.com/svera/coreander/v5/internal/webserver/model"
)

type fakeBookSource struct {
	mu        sync.Mutex
	ids       []string
	queries   []string
	book      biblioreads.BookResponse
	results   []biblioreads.SearchResult
	err       error
	searchErr error
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
	var bookSource document.BookSource
	idxCfg := index.Config{}
	if source != nil {
		bookSource = source
		idxCfg.GoodReadsSource = source
	}
	idx := index.NewBleve(documents, authors, appFS, testLibraryDir, nil, idxCfg)
	t.Cleanup(func() { _ = idx.Close() })

	for _, doc := range []index.Document{
		{
			ID: "linked.epub", Slug: "linked",
			Metadata:          metadata.Metadata{Title: "La Reina Dragon", Format: "EPUB"},
			GoodReadsBookURL:  "/book/show/12207444-la-reina-dragon",
			GoodReadsEnriched: true,
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
	controllers := webserver.SetupControllers(cfg, db, idx, sender, appFS, nil, bookSource)
	app := webserver.New(cfg, controllers, sender, idx, &model.UserRepository{DB: db})
	t.Cleanup(func() { _ = app.Shutdown() })
	return app, idx
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
	return page.Find(`li[hx-get$="/goodreads"]`)
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
	if url, _ := placeholder.Attr("hx-get"); url != "/documents/linked/goodreads" {
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
	if got := strings.TrimSpace(page.Find(`a[href="https://www.goodreads.com/book/show/1-unlinked"]`).Text()); got != "4.10" {
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
	if got := strings.TrimSpace(page.Find(`a[href="https://www.goodreads.com/book/show/1-unlinked"]`).Text()); got != "4.20" {
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
	if got := goodReadsPlaceholders(t, getBody(t, app, "/documents/unmatched")).Length(); got != 0 {
		t.Errorf("expected no placeholder for a document already looked up without match, got %d", got)
	}
	for _, path := range []string{"/documents/unmatched", "/documents/unmatched/goodreads"} {
		page, err := goquery.NewDocumentFromReader(strings.NewReader(getBody(t, app, path)))
		if err != nil {
			t.Fatal(err)
		}
		score := page.Find("li").FilterFunction(func(_ int, item *goquery.Selection) bool {
			return item.Find("i.bi-star-fill").Length() == 1
		})
		if strings.TrimSpace(score.Text()) != "No score" || score.Find("a").Length() != 0 {
			t.Errorf("%s: expected unlinked no-score row, got %q", path, score.Text())
		}
	}
	if len(source.queries) != 0 || len(source.ids) != 0 {
		t.Errorf("expected no calls, got queries %q and ids %q", source.queries, source.ids)
	}
}

func TestDocumentDetailGoodReadsRatingRendersNothingWhenUnavailable(t *testing.T) {
	for name, source := range map[string]*fakeBookSource{
		"error":     {err: errors.New("unavailable")},
		"no rating": {},
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
	if body := strings.TrimSpace(getBody(t, app, "/documents/linked/goodreads")); body != "" {
		t.Fatalf("expected no rating, got %q", body)
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
				if strings.TrimSpace(page.Find("li").Text()) != "No score" || page.Find("a").Length() != 0 {
					t.Errorf("expected no-score row without a link, got %q", body)
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
		{"en", "No score"},
		{"es", "Sin puntuación"},
		{"de", "Keine Bewertung"},
		{"fr", "Aucune note"},
		{"ru", "Нет оценки"},
	} {
		for _, path := range []string{"/documents/unmatched", "/documents/unmatched/goodreads"} {
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
