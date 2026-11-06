package biblioreads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/metadata"
)

func mustData(t *testing.T, document index.Document) DocumentData {
	t.Helper()
	data, err := index.DataSourceData[DocumentData](document, Source)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func withData(t *testing.T, document index.Document, data DocumentData) index.Document {
	t.Helper()
	if err := index.SetDataSourceData(&document, Source, data); err != nil {
		t.Fatal(err)
	}
	return document
}

func updateData(t *testing.T, document *index.Document, update func(*DocumentData)) {
	t.Helper()
	data := mustData(t, *document)
	update(&data)
	*document = withData(t, *document, data)
}

func assertNoDocumentAuthorURL(t *testing.T, documents bleve.Index) {
	t.Helper()
	req := bleve.NewSearchRequestOptions(bleve.NewMatchAllQuery(), 100, 0, false)
	req.Fields = []string{"*"}
	result, err := documents.Search(req)
	if err != nil {
		t.Fatal(err)
	}
	for _, hit := range result.Hits {
		if _, exists := hit.Fields["GoodReadsAuthorURL"]; exists {
			t.Errorf("document %s still stores an author URL", hit.ID)
		}
	}
}

type fakeSource struct {
	mu          sync.Mutex
	queries     []string
	results     []SearchResult
	byQuery     map[string][]SearchResult
	queryErrors map[string]error
	err         error
}

func (f *fakeSource) SearchBooks(_ context.Context, query string) (SearchBooksResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, query)
	if f.byQuery != nil {
		return SearchBooksResponse{Result: f.byQuery[query]}, f.queryErrors[query]
	}
	return SearchBooksResponse{Result: f.results}, f.err
}

func (f *fakeSource) Queries() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.queries)
}

type testReader struct{}

func (testReader) Metadata(string) (metadata.Metadata, error) {
	return metadata.Metadata{Title: "La Reina Dragon", Authors: []string{"Alice Borchardt", "Second Author"}, Format: "PDF"}, nil
}

func (testReader) Cover(string, int) (image.Image, error) {
	return nil, nil
}

var testResults = []SearchResult{
	{Title: "La Reina Dragon", Author: "Alice Borchardt", BookURL: "/book/show/12207444-la-reina-dragon", AuthorURL: "/author/show/151155.Alice_Borchardt", Rating: "3.82", RatingCount: "2,285"},
	{BookURL: "/book/show/2-other", AuthorURL: "/author/show/2.Other"},
}

const (
	wantDataSourceLink = "/book/show/12207444-la-reina-dragon"
	wantAuthorURL      = "/author/show/151155.Alice_Borchardt"
	testID             = "book.pdf"
)

type testIndexer struct {
	*index.BleveIndexer
	*Enricher
	documentsIdx *writeCountingIndex
	authorsIdx   bleve.Index
	fs           afero.Fs
}

func newTestIndexer(t *testing.T, source SearchSource) *testIndexer {
	t.Helper()
	documents, err := bleve.NewMemOnly(index.CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}
	authors, err := bleve.NewMemOnly(index.CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}
	writes := &writeCountingIndex{countedIndex: documents}
	adapter := NewEnricher(source)
	var providers []index.DocumentEnricher
	if source != nil {
		providers = append(providers, adapter)
	}
	appFS := afero.NewMemMapFs()
	if err := afero.WriteFile(appFS, "lib/book.pdf", []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	idx := index.NewBleve(writes, authors, appFS, "lib", map[string]metadata.Reader{".pdf": testReader{}}, index.Config{DocumentEnrichers: providers})
	adapter.SetIndex(idx)
	t.Cleanup(func() { _ = idx.Close() })
	return &testIndexer{BleveIndexer: idx, Enricher: adapter, documentsIdx: writes, authorsIdx: authors, fs: appFS}
}

func TestEnrichStoresMatchingResult(t *testing.T) {
	source := &fakeSource{results: testResults}
	idx := newTestIndexer(t, source)
	if err := idx.AddLibrary(10, true, 1); err != nil {
		t.Fatal(err)
	}

	if err := idx.EnrichDocuments(10); err != nil {
		t.Fatal(err)
	}

	doc, err := idx.DocumentByID(testID)
	if err != nil {
		t.Fatal(err)
	}
	if mustData(t, doc).URL != wantDataSourceLink {
		t.Errorf("unexpected book URL %q", mustData(t, doc).URL)
	}
	if doc.DataSourceScoreValue != 3.82 {
		t.Errorf("unexpected Goodreads rating %v", doc.DataSourceScoreValue)
	}
	if mustData(t, doc).RatingCount != "2,285" {
		t.Errorf("unexpected Goodreads rating count %q", mustData(t, doc).RatingCount)
	}
	assertNoDocumentAuthorURL(t, idx.documentsIdx)
	if !mustData(t, doc).Enriched {
		t.Error("expected document to be marked as BiblioReads-enriched")
	}
	author, err := idx.Author("alice-borchardt", "")
	if err != nil || author.DataSourceLinks["goodreads"] != wantAuthorURL {
		t.Fatalf("expected matching author URL stored, got %+v, %v", author, err)
	}
	other, err := idx.Author("second-author", "")
	if err != nil || other.DataSourceLinks["goodreads"] != "" {
		t.Fatalf("unmatched coauthor must not receive the URL, got %+v, %v", other, err)
	}
	if want := []string{"La Reina Dragon Alice Borchardt"}; !slices.Equal(source.Queries(), want) {
		t.Errorf("expected queries %q, got %q", want, source.Queries())
	}

	if err := idx.EnrichDocuments(10); err != nil {
		t.Fatal(err)
	}
	if len(source.Queries()) != 1 {
		t.Errorf("expected enriched documents not to be searched again, got queries %q", source.Queries())
	}
}

func TestEnrichMarksDocumentsWithoutMatch(t *testing.T) {
	for name, source := range map[string]*fakeSource{
		"no results":   {},
		"wrong title":  {results: []SearchResult{{Title: "Another book", Author: "Alice Borchardt", BookURL: "/book/show/1"}}},
		"wrong author": {results: []SearchResult{{Title: "La Reina Dragon", Author: "Another writer", BookURL: "/book/show/1"}}},
	} {
		t.Run(name, func(t *testing.T) {
			idx := newTestIndexer(t, source)
			if err := idx.AddLibrary(10, true, 1); err != nil {
				t.Fatal(err)
			}

			if err := idx.EnrichDocuments(10); err != nil {
				t.Fatal(err)
			}
			doc, err := idx.DocumentByID(testID)
			if err != nil {
				t.Fatal(err)
			}
			if !mustData(t, doc).Enriched || mustData(t, doc).URL != "" {
				t.Errorf("expected document enriched with no URLs, got %+v", doc)
			}
			if want := []string{"La Reina Dragon Alice Borchardt", "La Reina Dragon"}; !slices.Equal(source.Queries(), want) {
				t.Errorf("expected both searches before giving up, got %q", source.Queries())
			}
		})
	}
}

func TestEnrichStoresSecondAuthorURLAndPreservesItOnEdits(t *testing.T) {
	const authorURL = "/author/show/2"
	source := &fakeSource{results: []SearchResult{
		{Title: "La Reina Dragon", Author: "Second Author", BookURL: wantDataSourceLink, AuthorURL: authorURL},
	}}
	idx := newTestIndexer(t, source)
	if err := idx.AddLibrary(10, true, 1); err != nil {
		t.Fatal(err)
	}
	stale, err := idx.Author("second-author", "")
	if err != nil {
		t.Fatal(err)
	}
	stale.DataSourceLinks = map[string]string{"other": "https://example.org/second-author"}
	if err := idx.IndexAuthor(stale); err != nil {
		t.Fatal(err)
	}
	if err := idx.EnrichDocuments(10); err != nil {
		t.Fatal(err)
	}
	author, err := idx.Author("second-author", "")
	if err != nil || author.DataSourceLinks["goodreads"] != authorURL ||
		author.DataSourceLinks["other"] != "https://example.org/second-author" {
		t.Fatalf("expected second author URL stored, got %+v, %v", author, err)
	}
	first, err := idx.Author("alice-borchardt", "")
	if err != nil || first.DataSourceLinks["goodreads"] != "" {
		t.Fatalf("first author must not receive coauthor URL, got %+v, %v", first, err)
	}
	stale.DataSourceID = "Q2"
	stale.RetrievedOn = time.Now().UTC()
	stale.DataSourceLinks["goodreads"] = "/author/show/999.Stale"
	if err := idx.IndexAuthor(stale); err != nil {
		t.Fatal(err)
	}
	if stale.DataSourceLinks["goodreads"] != "/author/show/999.Stale" {
		t.Fatal("indexing an author mutated the caller's links")
	}
	author, err = idx.Author("second-author", "")
	if err != nil || author.DataSourceLinks["goodreads"] != authorURL ||
		author.DataSourceLinks["other"] != "https://example.org/second-author" ||
		author.DataSourceID != "Q2" || author.DocumentCount != 1 {
		t.Fatalf("stale author edit lost URL or metadata: %+v, %v", author, err)
	}
	if _, err := idx.NewFile("second.pdf", []byte("")); err != nil {
		t.Fatal(err)
	}
	author, err = idx.Author("second-author", "")
	if err != nil || author.DataSourceLinks["goodreads"] != authorURL || author.DocumentCount != 2 {
		t.Fatalf("incrementing document count lost author URL: %+v, %v", author, err)
	}
}

func TestSearchErrorsLeaveLookupPending(t *testing.T) {
	const title = "El señor de los anillos"
	const author = "J. R. R. Tolkien"
	searchError := errors.New("BiblioReads returned HTTP 404: upstream fetch or JSON failure")
	for _, failedQuery := range []string{title + " " + author, title} {
		t.Run(failedQuery, func(t *testing.T) {
			source := &fakeSource{
				byQuery:     map[string][]SearchResult{},
				queryErrors: map[string]error{failedQuery: searchError},
			}
			idx := newTestIndexer(t, source)
			document := index.Document{
				ID: testID, Metadata: metadata.Metadata{Title: title, Authors: []string{author}},
			}
			if err := index.IndexDocument(idx.documentsIdx, document.ID, document); err != nil {
				t.Fatal(err)
			}
			if _, err := idx.Book(context.Background(), document); !errors.Is(err, searchError) {
				t.Fatalf("expected wrapped search error, got %v", err)
			}
			stored, err := idx.DocumentByID(document.ID)
			if err != nil {
				t.Fatal(err)
			}
			if data := mustData(t, stored); data.Enriched || data.URL != "" {
				t.Fatalf("failed search must not persist a completed lookup: %+v", data)
			}
			source.queryErrors = nil
			source.byQuery[title] = []SearchResult{
				{Title: title, Author: author, BookURL: "/book/show/37910810"},
			}
			result, err := idx.Book(context.Background(), stored)
			if err != nil || result.BookURL != "/book/show/37910810" {
				t.Fatalf("expected successful retry, got %+v, %v", result, err)
			}
			stored, err = idx.DocumentByID(document.ID)
			if err != nil || !mustData(t, stored).Enriched {
				t.Fatalf("successful retry must persist completion: %+v, %v", stored, err)
			}
		})
	}
}

func TestEnrichLeavesDocumentsForLaterWhenUnavailable(t *testing.T) {
	source := &fakeSource{err: ErrUnavailable}
	idx := newTestIndexer(t, source)
	if err := idx.AddLibrary(10, true, 1); err != nil {
		t.Fatal(err)
	}
	if err := idx.EnrichDocuments(10); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	doc, err := idx.DocumentByID(testID)
	if err != nil {
		t.Fatal(err)
	}
	if mustData(t, doc).Enriched {
		t.Error("expected document not to be marked as BiblioReads-enriched")
	}

	source.mu.Lock()
	source.err = nil
	source.results = testResults
	source.mu.Unlock()
	if err := idx.EnrichDocuments(10); err != nil {
		t.Fatal(err)
	}
	if doc, _ = idx.DocumentByID(testID); mustData(t, doc).URL != wantDataSourceLink {
		t.Errorf("expected document to be enriched on retry, got %q", mustData(t, doc).URL)
	}
}

func TestEnrichBoundsNotFoundLookups(t *testing.T) {
	source := &fakeSource{err: fmt.Errorf("%w: %w", ErrUnavailable, ErrNotFound)}
	idx := newTestIndexer(t, source)
	doc := index.Document{ID: testID, Metadata: metadata.Metadata{Title: "El economista camuflado", Authors: []string{"Tim Harford"}}}
	if err := index.IndexDocument(idx.documentsIdx, doc.ID, doc); err != nil {
		t.Fatal(err)
	}
	for attempt := range 5 {
		err := idx.EnrichDocuments(1)
		if attempt < 3 && (!errors.Is(err, ErrNotFound) || !errors.Is(err, index.ErrEnrichmentNotFound)) {
			t.Fatalf("not-found classification must reach the retry scheduler: %v", err)
		}
		if attempt >= 3 && err != nil {
			t.Fatalf("exhausted lookup must not be retried automatically: %v", err)
		}
	}
	if queries := source.Queries(); !slices.Equal(queries, []string{
		"El economista camuflado Tim Harford",
		"El economista camuflado Tim Harford",
		"El economista camuflado Tim Harford",
	}) {
		t.Fatalf("expected exactly three attempts of the original query: %q", queries)
	}
	stored, err := idx.DocumentByID(doc.ID)
	if err != nil || mustData(t, stored).Enriched {
		t.Fatalf("exhausted lookups must remain unresolved: %+v, %v", stored, err)
	}
}

func TestNewFileSchedulesEnrichment(t *testing.T) {
	source := &fakeSource{results: testResults}
	idx := newTestIndexer(t, source)

	if _, err := idx.NewFile("new.pdf", []byte("")); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		doc, err := idx.DocumentByID("new.pdf")
		if err != nil {
			t.Fatal(err)
		}
		if mustData(t, doc).Enriched {
			if mustData(t, doc).URL != wantDataSourceLink {
				t.Errorf("unexpected book URL %q", mustData(t, doc).URL)
			}
			assertNoDocumentAuthorURL(t, idx.documentsIdx)
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for background BiblioReads enrichment")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestEnrichWithoutSourceIsNoop(t *testing.T) {
	idx := newTestIndexer(t, nil)
	if err := idx.AddLibrary(10, true, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.NewFile("new.pdf", []byte("")); err != nil {
		t.Fatal(err)
	}
	if err := idx.EnrichDocuments(10); err != nil {
		t.Fatal(err)
	}
	doc, err := idx.DocumentByID(testID)
	if err != nil {
		t.Fatal(err)
	}
	if mustData(t, doc).Enriched {
		t.Error("expected document not to be BiblioReads-enriched when the source is disabled")
	}
}

func TestEnrichSkipsSearchWhenDocumentHasLink(t *testing.T) {
	source := &fakeSource{results: testResults}
	idx := newTestIndexer(t, source)
	if err := idx.AddLibrary(10, true, 1); err != nil {
		t.Fatal(err)
	}
	doc, err := idx.DocumentByID(testID)
	if err != nil {
		t.Fatal(err)
	}
	updateData(t, &doc, func(data *DocumentData) {
		data.URL = "/book/show/1-existing"
	})
	if err := index.IndexDocument(idx.documentsIdx, doc.ID, doc); err != nil {
		t.Fatal(err)
	}

	if err := idx.EnrichDocuments(10); err != nil {
		t.Fatal(err)
	}

	if len(source.Queries()) != 0 {
		t.Errorf("expected no search for a document with a link, got %q", source.Queries())
	}
	doc, err = idx.DocumentByID(testID)
	if err != nil {
		t.Fatal(err)
	}
	if !mustData(t, doc).Enriched || mustData(t, doc).URL != "/book/show/1-existing" {
		t.Errorf("expected existing link kept and document marked enriched, got %+v", doc)
	}
}

func TestBook(t *testing.T) {
	source := &fakeSource{results: testResults}
	idx := newTestIndexer(t, source)
	if err := idx.AddLibrary(10, true, 1); err != nil {
		t.Fatal(err)
	}
	doc, err := idx.DocumentByID(testID)
	if err != nil {
		t.Fatal(err)
	}

	got, err := idx.Book(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if got.BookURL != wantDataSourceLink || got.Rating != "3.82" {
		t.Errorf("expected fresh book URL and search rating, got %+v", got)
	}
	stored, err := idx.DocumentByID(testID)
	if err != nil {
		t.Fatal(err)
	}
	if !mustData(t, stored).Enriched || mustData(t, stored).URL != wantDataSourceLink {
		t.Errorf("expected the link to be persisted, got %+v", stored)
	}

	if got, _ := idx.Book(context.Background(), stored); got.BookURL != wantDataSourceLink || got.Rating != "3.82" {
		t.Errorf("expected stored URL and rating, got %+v", got)
	}
	req := bleve.NewSearchRequest(bleve.NewDocIDQuery([]string{stored.ID}))
	req.Fields = []string{"*"}
	result, err := idx.documentsIdx.Search(req)
	if err != nil || len(result.Hits) != 1 {
		t.Fatalf("reading stored Goodreads fields: %v", err)
	}
	for _, field := range []string{"Rating", "rating"} {
		if _, exists := result.Hits[0].Fields[field]; exists {
			t.Errorf("rating must not be stored in document field %s", field)
		}
	}
	if err := idx.EnrichDocuments(10); err != nil {
		t.Fatal(err)
	}
	if len(source.Queries()) != 1 {
		t.Errorf("expected a single search, got %q", source.Queries())
	}
}

func TestParseRating(t *testing.T) {
	for _, tt := range []struct {
		rating string
		want   float64
	}{
		{rating: "0.00", want: 0},
		{rating: "3.82", want: 3.82},
		{rating: "5.01", want: 0},
		{rating: "unavailable", want: 0},
	} {
		t.Run(tt.rating, func(t *testing.T) {
			if got := parseRating(tt.rating, testID); got != tt.want {
				t.Errorf("parseBiblioReadsRating(%q) = %v, want %v", tt.rating, got, tt.want)
			}
		})
	}
}

func TestBookReturnsErrorWhenUnavailable(t *testing.T) {
	source := &fakeSource{err: ErrUnavailable}
	idx := newTestIndexer(t, source)
	if err := idx.AddLibrary(10, true, 1); err != nil {
		t.Fatal(err)
	}
	doc, _ := idx.DocumentByID(testID)
	if _, err := idx.Book(context.Background(), doc); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if doc, _ = idx.DocumentByID(testID); mustData(t, doc).Enriched {
		t.Error("expected document to stay unenriched for a later retry")
	}
}

func TestTitleMatchScore(t *testing.T) {
	for _, tt := range []struct {
		name      string
		title     string
		candidate string
		want      int
	}{
		{name: "normalized exact", title: "La promesa", candidate: "LA PROMESA!", want: 3},
		{name: "series suffix", title: "La promesa", candidate: "La promesa (Series, #1)", want: 3},
		{name: "document series suffix", title: "La promesa (Series, #1)", candidate: "La promesa", want: 3},
		{name: "subtitle", title: "La promesa", candidate: "La promesa: Una historia", want: 1},
		{name: "document subtitle", title: "La promesa: Una historia", candidate: "La promesa", want: 1},
		{name: "parenthetical", title: "La promesa", candidate: "La promesa (The Promise)", want: 1},
		{name: "different subtitles", title: "La promesa: Una historia", candidate: "La promesa: Otra historia"},
		{name: "standalone volume", title: "La promesa", candidate: "La promesa (#1)"},
		{name: "unrelated", title: "La promesa", candidate: "Otro libro"},
		{name: "empty candidate", title: "La promesa"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := titleMatchScore(tt.candidate, normalizedTitle(tt.title), mainTitle(tt.title))
			if got != tt.want {
				t.Fatalf("title match score = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestPreferBook(t *testing.T) {
	for _, tt := range []struct {
		name             string
		candidate, best  SearchResult
		score, bestScore int
		want             bool
	}{
		{name: "first match", score: 1, want: true},
		{name: "lower ID before rating for equal strength", candidate: SearchResult{BookURL: "/book/show/1"}, best: SearchResult{BookURL: "/book/show/2", Rating: "4"}, score: 3, bestScore: 3, want: true},
		{name: "higher ID loses despite rating", candidate: SearchResult{BookURL: "/book/show/2", Rating: "4"}, best: SearchResult{BookURL: "/book/show/1"}, score: 3, bestScore: 3},
		{name: "more ratings before ID", candidate: SearchResult{BookURL: "/book/show/2", RatingCount: "100"}, best: SearchResult{BookURL: "/book/show/1", RatingCount: "3"}, score: 3, bestScore: 3, want: true},
		{name: "fewer ratings loses", candidate: SearchResult{RatingCount: "3"}, best: SearchResult{RatingCount: "100"}, score: 3, bestScore: 3},
		{name: "result rank is not book ID", candidate: SearchResult{ID: 2, BookURL: "/book/show/1"}, best: SearchResult{ID: 1, BookURL: "/book/show/2"}, score: 3, bestScore: 3, want: true},
		{name: "rating before strength for different strength", candidate: SearchResult{ID: 2, Rating: "4"}, best: SearchResult{ID: 1}, score: 1, bestScore: 3, want: true},
		{name: "stronger unrated loses to rated", best: SearchResult{Rating: "4"}, score: 3, bestScore: 1},
		{name: "strength with same rating status", score: 3, bestScore: 1, want: true},
		{name: "weaker match loses with same rating status", score: 1, bestScore: 3},
		{name: "unknown ID falls back to rating", candidate: SearchResult{Rating: "4"}, best: SearchResult{ID: 1}, score: 3, bestScore: 3, want: true},
		{name: "same ID falls back to rating", candidate: SearchResult{ID: 1, Rating: "4"}, best: SearchResult{ID: 1}, score: 3, bestScore: 3, want: true},
		{name: "complete tie retains first match", candidate: SearchResult{ID: 1, Rating: "4"}, best: SearchResult{ID: 1, Rating: "3"}, score: 3, bestScore: 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := preferBook(tt.candidate, tt.best, tt.score, tt.bestScore); got != tt.want {
				t.Fatalf("candidate preference = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthorNameMatches(t *testing.T) {
	tests := []struct {
		name, candidate string
		want            bool
	}{
		{"Paul Kennedy", "Paul M. Kennedy", true},
		{"Paul Kennedy", "Paul Michael Kennedy", true},
		{"Paul Michael Kennedy", "Paul Michael James Kennedy", true},
		{"J. R. R. Tolkien", "J.R.R. Tolkien", true},
		{"JRR Tolkien", "J.R.R. Tolkien", true},
		{"José García", "JOSE M. GARCIA", true},
		{"Paul M. Kennedy", "Paul A. Kennedy", false},
		{"Paul Michael James Kennedy", "Paul James Michael Kennedy", false},
		{"Paul Kennedy", "Pauline Kennedy", false},
		{"Paul Kennedy", "Paul Kennedys", false},
		{"Paul Kennedy", "John Kennedy", false},
		{"Kennedy", "Paul M. Kennedy", false},
		{"Paul", "Paul M. Kennedy", false},
		{"", "Paul Kennedy", false},
		{" . ", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/"+tt.candidate, func(t *testing.T) {
			if got := authorNameMatches(tt.name, tt.candidate); got != tt.want {
				t.Errorf("match = %v, want %v", got, tt.want)
			}
			if got := authorNameMatches(tt.candidate, tt.name); got != tt.want {
				t.Errorf("reverse match = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthorPrefersExactAndRejectsAmbiguousPartialMatches(t *testing.T) {
	doc := index.Document{
		Metadata:     metadata.Metadata{Authors: []string{"Paul Kennedy", "Paul M. Kennedy"}},
		AuthorsSlugs: []string{"paul-kennedy", "paul-m-kennedy"},
	}
	if slug, name := matchedAuthor(doc, "Paul M. Kennedy"); slug != "paul-m-kennedy" || name != "Paul M. Kennedy" {
		t.Fatalf("expected exact author, got %q, %q", slug, name)
	}
	doc.Authors = []string{"Paul M. Kennedy", "Paul A. Kennedy"}
	if slug, name := matchedAuthor(doc, "Paul Kennedy"); slug != "" || name != "" {
		t.Fatalf("ambiguous partial match must not select an author, got %q, %q", slug, name)
	}
}

func TestBookMatchesKennedyMiddleName(t *testing.T) {
	const title = "Auge y caída de las grandes potencias"
	const bookURL = "/book/show/27214"
	const authorURL = "/author/show/8390124.Paul_M_Kennedy"
	source := &fakeSource{results: []SearchResult{{
		Title: title, Author: "Paul M. Kennedy", BookURL: bookURL, AuthorURL: authorURL, Rating: "4.14",
	}}}
	idx := newTestIndexer(t, source)
	doc := index.Document{
		ID: testID, AuthorsSlugs: []string{"paul-kennedy", "another-writer"},
		Metadata: metadata.Metadata{Title: title, Authors: []string{"Paul Kennedy", "Another Writer"}},
	}
	if err := index.IndexDocument(idx.documentsIdx, doc.ID, doc); err != nil {
		t.Fatal(err)
	}
	got, err := idx.Book(context.Background(), doc)
	if err != nil || got.BookURL != bookURL || got.Rating != "4.14" {
		t.Fatalf("expected Kennedy's book and rating, got %+v, %v", got, err)
	}
	stored, err := idx.DocumentByID(doc.ID)
	if err != nil || !mustData(t, stored).Enriched || mustData(t, stored).URL != bookURL || stored.DataSourceScoreValue != 4.14 {
		t.Fatalf("expected persisted book match and rating, got %+v, %v", stored, err)
	}
	author, err := idx.Author("paul-kennedy", "")
	if err != nil || author.DataSourceLinks["goodreads"] != authorURL || author.Name != "Paul Kennedy" {
		t.Fatalf("expected author URL with original name preserved, got %+v, %v", author, err)
	}
	other, err := idx.Author("another-writer", "")
	if err != nil || other.DataSourceLinks["goodreads"] != "" {
		t.Fatalf("unmatched coauthor must not receive the URL, got %+v, %v", other, err)
	}
	if want := []string{title + " Paul Kennedy"}; !slices.Equal(source.Queries(), want) {
		t.Fatalf("expected combined search to match without fallback, got %q", source.Queries())
	}
}

func TestMatchingBook(t *testing.T) {
	correct := SearchResult{
		Title: "El señor de los anillos", Author: "J.R.R. Tolkien",
		BookURL: "/book/show/37910810", AuthorURL: "/author/show/656983.J_R_R_Tolkien",
	}
	parody := SearchResult{
		Title: "El sopor de los anillos", Author: "The Harvard Lampoon",
		BookURL: "/book/show/7297403-el-sopor-de-los-anillos",
	}
	tests := []struct {
		name    string
		title   string
		authors []string
		results []SearchResult
		want    string
	}{
		{name: "correct result after parody", results: []SearchResult{parody, correct}, want: correct.BookURL},
		{name: "parody rejected", results: []SearchResult{parody}},
		{name: "similar title with correct author rejected", results: []SearchResult{{Title: parody.Title, Author: correct.Author, BookURL: parody.BookURL}}},
		{name: "wrong author rejected", results: []SearchResult{{Title: correct.Title, Author: parody.Author, BookURL: parody.BookURL}}},
		{name: "normalization", title: "  EL SEN\u0303OR DE LOS ANILLOS! ", results: []SearchResult{correct}, want: correct.BookURL},
		{name: "series suffix", results: []SearchResult{{Title: correct.Title + " (El señor de los anillos, #1)", Author: correct.Author, BookURL: correct.BookURL}}, want: correct.BookURL},
		{name: "document series suffix", title: correct.Title + " (Middle Earth, #1)", results: []SearchResult{correct}, want: correct.BookURL},
		{name: "omnibus range", results: []SearchResult{{Title: correct.Title + " (#1-#3)", Author: correct.Author, BookURL: correct.BookURL}}, want: correct.BookURL},
		{name: "omnibus range with dash", results: []SearchResult{{Title: correct.Title + " (#1–3)", Author: correct.Author, BookURL: correct.BookURL}}, want: correct.BookURL},
		{name: "document omnibus range", title: correct.Title + " (#1-#3)", results: []SearchResult{correct}, want: correct.BookURL},
		{name: "standalone volume retained", results: []SearchResult{{Title: correct.Title + " (#1)", Author: correct.Author, BookURL: correct.BookURL}}},
		{name: "series suffix ranks equally", results: []SearchResult{{Title: correct.Title + " (Middle Earth, #1)", Author: correct.Author, BookURL: "/book/show/2"}, correct}, want: "/book/show/2"},
		{name: "lower Goodreads ID preferred over rating for equal titles", results: []SearchResult{
			{ID: 1, Title: correct.Title, Author: correct.Author, BookURL: "/book/show/1-unrated", Rating: "0.00"},
			{ID: 2, Title: correct.Title, Author: correct.Author, BookURL: "/book/show/2-rated", Rating: "4.05"},
		}, want: "/book/show/1-unrated"},
		{name: "unrated exact match retained when no rated match exists", results: []SearchResult{
			{Title: correct.Title, Author: correct.Author, BookURL: "/book/show/1-unrated", Rating: "0.00"},
			{Title: correct.Title, Author: correct.Author, BookURL: "/book/show/2"},
		}, want: "/book/show/1-unrated"},
		{name: "colon subtitle", results: []SearchResult{{Title: correct.Title + ": Una historia", Author: correct.Author, BookURL: correct.BookURL}}, want: correct.BookURL},
		{name: "period subtitle", results: []SearchResult{{Title: correct.Title + ". Una historia", Author: correct.Author, BookURL: correct.BookURL}}, want: correct.BookURL},
		{name: "dash subtitle", results: []SearchResult{{Title: correct.Title + " — Una historia", Author: correct.Author, BookURL: correct.BookURL}}, want: correct.BookURL},
		{name: "document subtitle", title: correct.Title + ": Una historia", results: []SearchResult{correct}, want: correct.BookURL},
		{name: "different subtitles rejected", title: correct.Title + ": Primera historia", results: []SearchResult{{Title: correct.Title + ": Otra historia", Author: correct.Author, BookURL: correct.BookURL}}},
		{name: "subtitle wrong author rejected", results: []SearchResult{{Title: correct.Title + ": Una historia", Author: parody.Author, BookURL: correct.BookURL}}},
		{name: "exact preferred over subtitle", results: []SearchResult{{Title: correct.Title + ": Una historia", Author: correct.Author, BookURL: "/book/show/2"}, correct}, want: correct.BookURL},
		{name: "series preferred over subtitle", results: []SearchResult{{Title: correct.Title + ": Una historia", Author: correct.Author, BookURL: "/book/show/2"}, {Title: correct.Title + " (#1-#3)", Author: correct.Author, BookURL: correct.BookURL}}, want: correct.BookURL},
		{name: "arbitrary prefix rejected", results: []SearchResult{{Title: correct.Title + " y otras historias", Author: correct.Author, BookURL: correct.BookURL}}},
		{name: "volume retained", results: []SearchResult{{Title: correct.Title + " 2", Author: correct.Author, BookURL: correct.BookURL}}},
		{name: "trailing parenthetical accepted as weaker match", results: []SearchResult{{Title: correct.Title + " (An edition)", Author: correct.Author, BookURL: correct.BookURL}}, want: correct.BookURL},
		{name: "missing result author", results: []SearchResult{{Title: correct.Title, BookURL: correct.BookURL}}},
		{name: "missing result title", results: []SearchResult{{Author: correct.Author, BookURL: correct.BookURL}}},
		{name: "missing URL", results: []SearchResult{{Title: correct.Title, Author: correct.Author}}},
		{name: "any document author", authors: []string{"Another Writer", "J. R. R. Tolkien"}, results: []SearchResult{correct}, want: correct.BookURL},
		{name: "no author", authors: []string{}, results: []SearchResult{correct}},
		{name: "no results"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title, authors := tt.title, tt.authors
			if title == "" {
				title = correct.Title
			}
			if authors == nil {
				authors = []string{"J. R. R. Tolkien"}
			}
			match, ok := matchingBook(index.Document{Metadata: metadata.Metadata{Title: title, Authors: authors}}, tt.results)
			if match.BookURL != tt.want || ok != (tt.want != "") {
				t.Fatalf("got %q, %v; want %q", match.BookURL, ok, tt.want)
			}
		})
	}
}

func TestMatchingBookPrefersMoreRatingsWithSeriesSuffix(t *testing.T) {
	document := index.Document{Metadata: metadata.Metadata{
		Title: "Los asesinatos de Manhattan", Authors: []string{"Douglas Preston", "Lincoln Child"},
	}}
	popular := SearchResult{
		ID: 1, Title: "Los asesinatos de Manhattan (Pendergast, #3)", Author: "Douglas Preston",
		BookURL: "/book/show/67005.Los_asesinatos_de_Manhattan", Rating: "4.26", RatingCount: "57,469",
	}
	obscure := SearchResult{
		ID: 3, Title: document.Title, Author: "Douglas Preston",
		BookURL: "/book/show/152308668-los-asesinatos-de-manhattan", Rating: "3.67", RatingCount: "3",
	}
	for _, results := range [][]SearchResult{{popular, obscure}, {obscure, popular}} {
		match, ok := matchingBook(document, results)
		if !ok || match.BookURL != popular.BookURL {
			t.Fatalf("expected popular series entry, got %+v, %v", match, ok)
		}
	}
	popular.Author = "Another author"
	match, ok := matchingBook(document, []SearchResult{popular, obscure})
	if !ok || match.BookURL != obscure.BookURL {
		t.Fatalf("rating count must not bypass author matching, got %+v, %v", match, ok)
	}
}

func TestDataSourceScoreCount(t *testing.T) {
	for _, tt := range []struct {
		value string
		want  uint64
	}{
		{"", 0}, {"0", 0}, {"57469", 57469}, {" 57,469 ", 57469},
		{"unknown", 0}, {"-1", 0}, {"3.5", 0}, {"18446744073709551616", 0},
	} {
		t.Run(tt.value, func(t *testing.T) {
			if got := ratingCount(tt.value); got != tt.want {
				t.Fatalf("rating count = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestMatchingBookPrefersLowerIDForEqualTitle(t *testing.T) {
	const olderBookURL = "/book/show/1368314.El_Cercano_Oriente"
	const newerBookURL = "/book/show/243175949-el-cercano-oriente"
	document := index.Document{Metadata: metadata.Metadata{
		Title: "El cercano Oriente", Authors: []string{"Isaac Asimov"},
	}}
	older := SearchResult{
		ID: 2, Title: "El Cercano Oriente", Author: "Isaac Asimov",
		BookURL: olderBookURL, Rating: "4.10",
	}
	newer := SearchResult{
		ID: 1, Title: "El Cercano Oriente", Author: "Isaac Asimov",
		BookURL: newerBookURL, Rating: "4.26",
	}

	for _, results := range [][]SearchResult{{newer, older}, {older, newer}} {
		match, ok := matchingBook(document, results)
		if !ok || match.BookURL != olderBookURL {
			t.Fatalf("expected lower Goodreads ID %q, got %+v, %v", olderBookURL, match, ok)
		}
	}
}

func TestMatchingBookPrefersRatedParentheticalTitle(t *testing.T) {
	doc := index.Document{Metadata: metadata.Metadata{Title: "La promesa", Authors: []string{"Pearl S. Buck"}}}
	rated := SearchResult{
		Title: "La promesa (The Promise)", Author: "Pearl S. Buck",
		BookURL: "/book/show/7377104-la-promesa", Rating: "4.04",
	}
	unrated := SearchResult{
		Title: "La promesa", Author: "Pearl S. Buck",
		BookURL: "/book/show/210898619-la-promesa", Rating: "0.00",
	}
	for _, results := range [][]SearchResult{{rated, unrated}, {unrated, rated}} {
		match, ok := matchingBook(doc, results)
		if !ok || match.BookURL != rated.BookURL || match.Rating != rated.Rating {
			t.Fatalf("expected rated translated-title entry, got %+v, %v", match, ok)
		}
	}
	rated.Author = "Another author"
	match, ok := matchingBook(doc, []SearchResult{rated, unrated})
	if !ok || match.BookURL != unrated.BookURL {
		t.Fatalf("rated entry by another author must not win, got %+v, %v", match, ok)
	}
}

func TestBookMatchesDolnickSubtitle(t *testing.T) {
	const title = "La escritura de los dioses"
	const author = "Edward Dolnick"
	const bookURL = "/book/show/208277727-la-escritura-de-los-dioses-descifrando-la-piedra-de-rosetta"
	source := &fakeSource{results: []SearchResult{{
		Title: title + ". Descifrando la piedra de Rosetta", Author: author,
		BookURL: bookURL, Rating: "4.19",
	}}}
	idx := newTestIndexer(t, source)
	doc := index.Document{
		ID: testID, AuthorsSlugs: []string{"edward-dolnick"},
		Metadata: metadata.Metadata{Title: title, Authors: []string{author}},
	}
	if err := index.IndexDocument(idx.documentsIdx, doc.ID, doc); err != nil {
		t.Fatal(err)
	}
	got, err := idx.Book(context.Background(), doc)
	if err != nil || got.BookURL != bookURL || got.Rating != "4.19" {
		t.Fatalf("expected Dolnick's book and score, got %+v, %v", got, err)
	}
	stored, err := idx.DocumentByID(doc.ID)
	if err != nil || !mustData(t, stored).Enriched || mustData(t, stored).URL != bookURL {
		t.Fatalf("expected subtitle match persisted, got %+v, %v", stored, err)
	}
	if want := []string{title + " " + author}; !slices.Equal(source.Queries(), want) {
		t.Fatalf("expected combined title/author search to match, got %q", source.Queries())
	}
}

func TestTitleFallback(t *testing.T) {
	const title = "El señor de los anillos"
	correct := SearchResult{Title: title, Author: "J.R.R. Tolkien", BookURL: "/book/show/37910810"}
	parody := SearchResult{Title: "El sopor de los anillos", Author: "The Harvard Lampoon", BookURL: "/book/show/7297403"}
	for _, fallbackUnavailable := range []bool{false, true} {
		name := "matched"
		if fallbackUnavailable {
			name = "unavailable"
		}
		t.Run(name, func(t *testing.T) {
			source := &fakeSource{byQuery: map[string][]SearchResult{
				title + " J. R. R. Tolkien": {parody},
				title:                       {parody, correct},
			}}
			idx := newTestIndexer(t, source)
			doc := index.Document{ID: testID, AuthorsSlugs: []string{"j-r-r-tolkien"}, Metadata: metadata.Metadata{Title: title, Authors: []string{"J. R. R. Tolkien"}}}
			if err := index.IndexDocument(idx.documentsIdx, doc.ID, doc); err != nil {
				t.Fatal(err)
			}
			if fallbackUnavailable {
				source.queryErrors = map[string]error{title: ErrUnavailable}
			}
			got, err := idx.Book(context.Background(), doc)
			if fallbackUnavailable {
				if !errors.Is(err, ErrUnavailable) || got.BookURL != "" {
					t.Fatalf("expected unavailable error, got %+v, %v", got, err)
				}
				stored, readErr := idx.DocumentByID(doc.ID)
				if readErr != nil || mustData(t, stored).Enriched {
					t.Fatalf("expected document left for retry, got %+v, %v", stored, readErr)
				}
				source.queryErrors = nil
				got, err = idx.Book(context.Background(), stored)
			}
			if err != nil || got.BookURL != correct.BookURL {
				t.Fatalf("got %+v, %v; want %q", got, err, correct.BookURL)
			}
			want := []string{title + " J. R. R. Tolkien", title}
			if fallbackUnavailable {
				want = append(want, want...)
			}
			if !slices.Equal(source.Queries(), want) {
				t.Fatalf("queries = %q, want %q", source.Queries(), want)
			}
		})
	}
}

func TestUnavailableSearchDoesNotBroadenQuery(t *testing.T) {
	const title = "El economista camuflado"
	const author = "Tim Harford"
	source := &fakeSource{
		byQuery: map[string][]SearchResult{
			title: {{Title: title, Author: author, BookURL: "/book/show/123", Rating: "4.00"}},
		},
		queryErrors: map[string]error{title + " " + author: ErrUnavailable},
	}
	idx := newTestIndexer(t, source)
	doc := index.Document{ID: testID, Metadata: metadata.Metadata{Title: title, Authors: []string{author}}}
	if err := index.IndexDocument(idx.documentsIdx, doc.ID, doc); err != nil {
		t.Fatal(err)
	}
	got, err := idx.Book(context.Background(), doc)
	if !errors.Is(err, ErrUnavailable) || got.BookURL != "" {
		t.Fatalf("expected unavailable search without a match, got %+v, %v", got, err)
	}
	stored, err := idx.DocumentByID(doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if mustData(t, stored).Enriched {
		t.Fatal("failed search must remain pending")
	}
	if !slices.Equal(source.Queries(), []string{title + " " + author}) {
		t.Fatalf("failed search must not trigger a title-only query, got %q", source.Queries())
	}
}

func TestMatchesTolkienOmnibusSearchResults(t *testing.T) {
	const title = "El señor de los anillos"
	const wantURL = "/book/show/37910810-el-se-or-de-los-anillos"
	// Titles and ordering from ' actual title-only search response.
	results := []SearchResult{
		{Title: "La comunidad del anillo (El señor de los anillos, #1)", Author: "J.R.R. Tolkien", BookURL: "/book/show/222947.La_comunidad_del_anillo"},
		{Title: "Las dos torres (El señor de los anillos, #2)", Author: "J.R.R. Tolkien", BookURL: "/book/show/15280.Las_dos_torres"},
		{Title: "El retorno del rey (El señor de los anillos, #3)", Author: "J.R.R. Tolkien", BookURL: "/book/show/63358.El_retorno_del_rey"},
		{Title: "El señor de los anillos (#1-#3)", Author: "J.R.R. Tolkien", BookURL: wantURL, AuthorURL: "/author/show/656983.J_R_R_Tolkien"},
		{Title: "Apéndices (El Señor de los Anillos, #4)", Author: "J.R.R. Tolkien", BookURL: "/book/show/54694.Ap_ndices"},
	}
	source := &fakeSource{byQuery: map[string][]SearchResult{title: results}}
	idx := newTestIndexer(t, source)
	doc := index.Document{ID: testID, AuthorsSlugs: []string{"j-r-r-tolkien"}, Metadata: metadata.Metadata{Title: title, Authors: []string{"J. R. R. Tolkien"}}}
	if err := index.IndexDocument(idx.documentsIdx, doc.ID, doc); err != nil {
		t.Fatal(err)
	}
	got, err := idx.Book(context.Background(), doc)
	if err != nil || got.BookURL != wantURL {
		t.Fatalf("got %+v, %v; want %q", got, err, wantURL)
	}
	stored, err := idx.DocumentByID(doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !mustData(t, stored).Enriched || mustData(t, stored).URL != wantURL {
		t.Fatalf("expected persisted omnibus match, got %+v", stored)
	}
	author, err := idx.Author("j-r-r-tolkien", "")
	if err != nil || author.DataSourceLinks["goodreads"] != results[3].AuthorURL {
		t.Fatalf("expected author URL on author record, got %+v, %v", author, err)
	}
	if want := []string{title + " J. R. R. Tolkien", title}; !slices.Equal(source.Queries(), want) {
		t.Fatalf("queries = %q, want %q", source.Queries(), want)
	}
}

func TestForceIndexingRefreshesMatch(t *testing.T) {
	source := &fakeSource{results: testResults}
	idx := newTestIndexer(t, source)
	if err := idx.AddLibrary(10, true, 1); err != nil {
		t.Fatal(err)
	}
	doc, err := idx.DocumentByID(testID)
	if err != nil {
		t.Fatal(err)
	}
	updateData(t, &doc, func(data *DocumentData) {
		data.URL = "/book/show/1-wrong"
		data.ResourceID = "wrong-work"
		data.LegacyBookID = 1
		data.Enriched = true
	})
	if err := index.IndexDocument(idx.documentsIdx, doc.ID, doc); err != nil {
		t.Fatal(err)
	}
	if err := idx.AddLibrary(10, true, 1); err != nil {
		t.Fatal(err)
	}
	if err := idx.EnrichDocuments(10); err != nil {
		t.Fatal(err)
	}
	doc, err = idx.DocumentByID(testID)
	if err != nil {
		t.Fatal(err)
	}
	if mustData(t, doc).URL != wantDataSourceLink {
		t.Fatalf("expected refreshed book link, got %q", mustData(t, doc).URL)
	}
	if mustData(t, doc).ResourceID != "" || mustData(t, doc).LegacyBookID != 0 {
		t.Fatalf("expected old identifiers to be cleared, got %q and %d", mustData(t, doc).ResourceID, mustData(t, doc).LegacyBookID)
	}
}

func TestStoreDetails(t *testing.T) {
	for _, scenario := range []string{"linked", "newly linked", "changed metadata", "changed match", "deleted", "partial identifiers"} {
		t.Run(scenario, func(t *testing.T) {
			idx := newTestIndexer(t, nil)
			doc := withData(t, index.Document{
				ID:       testID,
				Metadata: metadata.Metadata{Title: "La Reina Dragon", Authors: []string{"Alice Borchardt"}},

				TextRankWords: []string{"dragon"},
			}, DocumentData{URL: wantDataSourceLink, Enriched: true})

			stale := doc
			resourceID := "work-id"
			switch scenario {
			case "newly linked":
				updateData(t, &stale, func(data *DocumentData) {
					data.URL = ""
					data.Enriched = false
				})
			case "changed metadata":
				doc.Title = "Another title"
			case "changed match":
				updateData(t, &doc, func(data *DocumentData) {
					data.URL = "/book/show/2-other"
				})
			case "partial identifiers":
				resourceID = ""
			}
			if scenario != "deleted" {
				if err := index.IndexDocument(idx.documentsIdx, doc.ID, doc); err != nil {
					t.Fatal(err)
				}
			}

			book := BookResponse{Rating: "4.12", ResourceID: resourceID, LegacyBookID: 12207444}
			writes := idx.documentsIdx
			writes.count.Store(0)
			err := idx.StoreDetails(stale, wantDataSourceLink, book)
			wantError := scenario != "linked" && scenario != "newly linked" && scenario != "partial identifiers"
			if (err != nil) != wantError {
				t.Fatalf("StoreDetails error = %v, want error %v", err, wantError)
			}
			if scenario == "deleted" {
				if !errors.Is(err, index.ErrDocumentNotFound) {
					t.Fatalf("expected index.ErrDocumentNotFound, got %v", err)
				}
				return
			}
			stored, err := idx.DocumentByID(doc.ID)
			if err != nil {
				t.Fatal(err)
			}
			if wantError {
				if writes.count.Load() != 0 {
					t.Fatal("stale details must not cause any index writes")
				}
				if mustData(t, stored).ResourceID != "" || mustData(t, stored).LegacyBookID != 0 || stored.DataSourceScoreValue != 0 {
					t.Fatalf("unexpected details persisted: %+v", stored)
				}
				return
			}
			wantLegacyBookID := 12207444
			if scenario == "partial identifiers" {
				wantLegacyBookID = 0
			}
			if mustData(t, stored).ResourceID != resourceID || mustData(t, stored).LegacyBookID != wantLegacyBookID {
				t.Fatalf("identifiers not persisted: %+v", stored)
			}
			if stored.DataSourceScoreValue != 4.12 {
				t.Fatalf("rating not persisted with identifiers: %+v", stored)
			}
			if !mustData(t, stored).Enriched || !slices.Equal(stored.TextRankWords, doc.TextRankWords) {
				t.Fatalf("existing enrichment was lost: %+v", stored)
			}
			book.Rating = "4.120"
			if err := idx.StoreDetails(stale, wantDataSourceLink, book); err != nil {
				t.Fatalf("repeated store failed: %v", err)
			}
			if writes.count.Load() != 1 {
				t.Fatalf("expected one combined write and no write for unchanged details, got %d", writes.count.Load())
			}
		})
	}
}

type countedIndex interface {
	bleve.Index
}

type writeCountingIndex struct {
	countedIndex
	count atomic.Int64
}

func (i *writeCountingIndex) Index(id string, data any) error {
	i.count.Add(1)
	return i.countedIndex.Index(id, data)
}

func TestStoreDetailsPreservesMissingFields(t *testing.T) {
	for _, tt := range []struct {
		name       string
		book       BookResponse
		wantRating string
		wantCount  string
		wantWork   string
		wantID     int
	}{
		{name: "rating only", book: BookResponse{Rating: "4.12"}, wantRating: "4.12", wantWork: "old-work", wantID: 1},
		{name: "identifiers only", book: BookResponse{ResourceID: "new-work", LegacyBookID: 2}, wantRating: "3.82", wantWork: "new-work", wantID: 2},
		{name: "missing work", book: BookResponse{Rating: "4.12", LegacyBookID: 2}, wantRating: "4.12", wantWork: "old-work", wantID: 1},
		{name: "missing edition", book: BookResponse{Rating: "4.12", ResourceID: "new-work"}, wantRating: "4.12", wantWork: "old-work", wantID: 1},
		{name: "empty response", wantRating: "3.82", wantWork: "old-work", wantID: 1},
		{name: "count only", book: BookResponse{RatingCount: "650"}, wantRating: "3.82", wantCount: "650", wantWork: "old-work", wantID: 1},
		{name: "zero count", book: BookResponse{RatingCount: "0"}, wantRating: "3.82", wantCount: "0", wantWork: "old-work", wantID: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			idx := newTestIndexer(t, nil)
			doc := withData(t, index.Document{
				ID:                   testID,
				DataSourceScoreValue: 3.82,
			}, DocumentData{URL: wantDataSourceLink, RatingCount: "2,285", ResourceID: "old-work", LegacyBookID: 1})

			if err := index.IndexDocument(idx.documentsIdx, doc.ID, doc); err != nil {
				t.Fatal(err)
			}
			if err := idx.StoreDetails(doc, mustData(t, doc).URL, tt.book); err != nil {
				t.Fatal(err)
			}
			stored, err := idx.DocumentByID(doc.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.DataSourceScoreValue != parseRating(tt.wantRating, doc.ID) || mustData(t, stored).ResourceID != tt.wantWork || mustData(t, stored).LegacyBookID != tt.wantID {
				t.Fatalf("partial details changed fields that were not supplied: %+v", stored)
			}
			wantCount := tt.wantCount
			if wantCount == "" {
				wantCount = mustData(t, doc).RatingCount
			}
			if mustData(t, stored).RatingCount != wantCount {
				t.Fatalf("expected rating count %q, got %q", wantCount, mustData(t, stored).RatingCount)
			}
		})
	}
}

func TestUpdateDataSourceLink(t *testing.T) {
	idx := newTestIndexer(t, nil)
	doc := withData(t, index.Document{
		ID: testID, Slug: "la-reina-dragon",
		Metadata:      metadata.Metadata{Title: "La Reina Dragon", Authors: []string{"Alice Borchardt"}},
		TextRankWords: []string{"dragon"},

		DataSourceScoreValue: 3.82,
	}, DocumentData{URL: wantDataSourceLink, RatingCount: "2,285", ResourceID: "old-work", LegacyBookID: 12207444, Enriched: true})

	if err := index.IndexDocument(idx.documentsIdx, doc.ID, doc); err != nil {
		t.Fatal(err)
	}
	const newLink = "provider-specific-document-reference"
	if err := idx.UpdateDataSourceLink(doc.ID, newLink); err != nil {
		t.Fatal(err)
	}
	stored, err := idx.DocumentByID(doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if mustData(t, stored).URL != newLink || !mustData(t, stored).Enriched ||
		stored.DataSourceScoreValue != 0 || mustData(t, stored).RatingCount != "" || mustData(t, stored).ResourceID != "" || mustData(t, stored).LegacyBookID != 0 {
		t.Fatalf("updating review link must clear cached score and provider lookup data: %+v", stored)
	}
	if stored.Title != doc.Title || stored.Slug != doc.Slug || !slices.Equal(stored.TextRankWords, doc.TextRankWords) {
		t.Fatalf("review-link update changed unrelated document data: %+v", stored)
	}
	if err := idx.StoreDetails(doc, mustData(t, doc).URL, BookResponse{Rating: "4.5"}); err == nil {
		t.Fatal("stale score write must not overwrite the updated link")
	}
}

func TestUpdateDataSourceLinkMissingDocument(t *testing.T) {
	idx := newTestIndexer(t, nil)
	if err := idx.UpdateDataSourceLink("missing", "provider-specific-document-reference"); !errors.Is(err, index.ErrDocumentNotFound) {
		t.Fatalf("expected missing-document error, got %v", err)
	}
}

func TestDataSourcesSurviveProviderAndStaleWrites(t *testing.T) {
	idx := newTestIndexer(t, &fakeSource{results: testResults})
	if err := idx.AddLibrary(10, true, 1); err != nil {
		t.Fatal(err)
	}
	stale, err := idx.DocumentByID(testID)
	if err != nil {
		t.Fatal(err)
	}
	current := stale
	other := map[string]any{"id": "external-id", "nested": map[string]any{"count": float64(3)}}
	if err := index.SetDataSourceData(&current, "other", other); err != nil {
		t.Fatal(err)
	}
	if _, exists := stale.DataSources["other"]; exists {
		t.Fatal("updating a provider mutated another document's map")
	}
	if err := index.IndexDocument(idx.documentsIdx, current.ID, current); err != nil {
		t.Fatal(err)
	}
	if err := idx.Enricher.Enrich(context.Background(), idx.BleveIndexer, stale); err != nil {
		t.Fatal(err)
	}
	if err := idx.StoreDetails(stale, wantDataSourceLink, BookResponse{
		Rating: "4.5", RatingCount: "100", ResourceID: "work", LegacyBookID: 123,
	}); err != nil {
		t.Fatal(err)
	}
	if err := idx.UpdateDataSourceLink(current.ID, "/book/show/456"); err != nil {
		t.Fatal(err)
	}
	stale.TextRankWords = []string{"updated"}
	if err := idx.UpdateDocument(stale.ID, func(update *index.DocumentUpdate) (bool, error) {
		update.TextRankWords = stale.TextRankWords
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	got, err := idx.DocumentByID(stale.ID)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := index.DataSourceData[map[string]any](got, "other")
	if err != nil || !reflect.DeepEqual(decoded, other) {
		t.Fatalf("another provider's payload was lost: %+v, %v", decoded, err)
	}
	if data := mustData(t, got); data != (DocumentData{URL: "/book/show/456", Enriched: true}) {
		t.Fatalf("stale write restored old Goodreads state: %+v", data)
	}
}

func TestBookRejectsInvalidProviderData(t *testing.T) {
	idx := newTestIndexer(t, nil)
	document := index.Document{DataSources: map[string]json.RawMessage{Source: json.RawMessage(`{"url":42}`)}}
	if _, err := idx.Book(context.Background(), document); err == nil {
		t.Fatal("Goodreads accepted an incorrectly typed URL")
	}
}

type blockingSource struct {
	started chan struct{}
	release chan struct{}
}

func (s *blockingSource) SearchBooks(ctx context.Context, _ string) (SearchBooksResponse, error) {
	s.started <- struct{}{}
	select {
	case <-s.release:
		return SearchBooksResponse{}, nil
	case <-ctx.Done():
		return SearchBooksResponse{}, ctx.Err()
	}
}

func TestEnrichLimitsConcurrentLookupsAcrossCallers(t *testing.T) {
	const callers = 2 * maxConcurrentLookups
	source := &blockingSource{started: make(chan struct{}, callers), release: make(chan struct{})}
	idx := newTestIndexer(t, source)
	var callersDone sync.WaitGroup
	defer func() {
		close(source.release)
		callersDone.Wait()
	}()
	// Separate callers model the batch and upload pools sharing one adapter.
	for range callers {
		callersDone.Go(func() {
			_, _ = idx.enrich(context.Background(), idx.BleveIndexer, index.Document{Metadata: metadata.Metadata{Title: "Title"}})
		})
	}
	for range maxConcurrentLookups {
		select {
		case <-source.started:
		case <-time.After(time.Second):
			t.Fatal("expected concurrent lookups")
		}
	}
	select {
	case <-source.started:
		t.Fatal("lookups exceeded the adapter concurrency limit")
	case <-time.After(30 * time.Millisecond):
	}
}
