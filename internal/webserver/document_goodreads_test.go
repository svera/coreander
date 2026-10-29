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
	mu      sync.Mutex
	ids     []string
	queries []string
	book    biblioreads.BookResponse
	results []biblioreads.SearchResult
	err     error
}

func (f *fakeBookSource) SearchBooks(_ context.Context, query string) (biblioreads.SearchBooksResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, query)
	return biblioreads.SearchBooksResponse{Result: f.results}, nil
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
	response, err := app.Test(mustGetRequest(t, path))
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
		book:    biblioreads.BookResponse{Rating: "4.10", ResourceID: "work-id", LegacyBookID: 1},
		results: []biblioreads.SearchResult{{Title: "Unlinked", Author: "Jane Doe", BookURL: "/book/show/1-unlinked", AuthorURL: "/author/show/1.Jane_Doe"}, {BookURL: "/book/show/2-other"}},
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
	if len(source.ids) != 1 || source.ids[0] != "1-unlinked" {
		t.Errorf("expected Book to be called with the found id, got %q", source.ids)
	}

	stored, err := idx.Document("unlinked")
	if err != nil {
		t.Fatal(err)
	}
	if stored.GoodReadsBookURL != "/book/show/1-unlinked" || stored.GoodReadsAuthorURL != "/author/show/1.Jane_Doe" || !stored.GoodReadsEnriched {
		t.Errorf("expected the found link to be stored, got %+v", stored)
	}
	if stored.GoodReadsResourceID != "work-id" || stored.GoodReadsLegacyBookID != 1 {
		t.Fatalf("expected identifiers for newly linked document to be stored, got %+v", stored)
	}

	if err := idx.EnrichGoodReads(10); err != nil {
		t.Fatal(err)
	}
	getBody(t, app, "/documents/unlinked/goodreads")
	if len(source.queries) != 1 {
		t.Errorf("expected the stored link to be reused without searching again, got %q", source.queries)
	}
}

func TestDocumentDetailGoodReadsSkipsUnmatchedDocuments(t *testing.T) {
	source := &fakeBookSource{book: biblioreads.BookResponse{Rating: "3.82"}}
	app, _ := newGoodReadsTestApp(t, source)
	if got := goodReadsPlaceholders(t, getBody(t, app, "/documents/unmatched")).Length(); got != 0 {
		t.Errorf("expected no placeholder for a document already looked up without match, got %d", got)
	}
	if body := strings.TrimSpace(getBody(t, app, "/documents/unmatched/goodreads")); body != "" {
		t.Errorf("expected empty response, got %q", body)
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
	for _, slug := range []string{"linked", "unlinked"} {
		if got := goodReadsPlaceholders(t, getBody(t, app, "/documents/"+slug)).Length(); got != 0 {
			t.Errorf("%s: expected no placeholder when GoodReads is disabled, got %d", slug, got)
		}
		if body := strings.TrimSpace(getBody(t, app, "/documents/"+slug+"/goodreads")); body != "" {
			t.Errorf("%s: expected empty response, got %q", slug, body)
		}
	}
}
