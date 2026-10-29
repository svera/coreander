package index

import (
	"context"
	"errors"
	"image"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
	"github.com/svera/coreander/v5/internal/metadata"
)

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

type fakeGoodReadsSource struct {
	mu          sync.Mutex
	queries     []string
	results     []biblioreads.SearchResult
	byQuery     map[string][]biblioreads.SearchResult
	queryErrors map[string]error
	err         error
}

func (f *fakeGoodReadsSource) SearchBooks(_ context.Context, query string) (biblioreads.SearchBooksResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, query)
	if f.byQuery != nil {
		return biblioreads.SearchBooksResponse{Result: f.byQuery[query]}, f.queryErrors[query]
	}
	return biblioreads.SearchBooksResponse{Result: f.results}, f.err
}

func (f *fakeGoodReadsSource) Queries() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.queries)
}

type goodReadsTestReader struct{}

func (goodReadsTestReader) Metadata(string) (metadata.Metadata, error) {
	return metadata.Metadata{Title: "La Reina Dragon", Authors: []string{"Alice Borchardt", "Second Author"}, Format: "PDF"}, nil
}

func (goodReadsTestReader) Cover(string, int) (image.Image, error) {
	return nil, nil
}

var goodReadsTestResults = []biblioreads.SearchResult{
	{Title: "La Reina Dragon", Author: "Alice Borchardt", BookURL: "/book/show/12207444-la-reina-dragon", AuthorURL: "/author/show/151155.Alice_Borchardt", Rating: "3.82"},
	{BookURL: "/book/show/2-other", AuthorURL: "/author/show/2.Other"},
}

const (
	wantGoodReadsBookURL   = "/book/show/12207444-la-reina-dragon"
	wantGoodReadsAuthorURL = "/author/show/151155.Alice_Borchardt"
	goodReadsTestID        = "book.pdf"
)

func newGoodReadsTestIndexer(t *testing.T, source GoodReadsSource) *BleveIndexer {
	t.Helper()
	idx, _ := newTestTextRankIndexer(t, Config{GoodReadsSource: source, GoodReadsEnrichInterval: time.Millisecond})
	appFS := afero.NewMemMapFs()
	if err := afero.WriteFile(appFS, "lib/book.pdf", []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	idx.fs = appFS
	idx.libraryPath = "lib"
	idx.reader = map[string]metadata.Reader{".pdf": goodReadsTestReader{}}
	return idx
}

func TestEnrichGoodReadsStoresMatchingResult(t *testing.T) {
	source := &fakeGoodReadsSource{results: goodReadsTestResults}
	idx := newGoodReadsTestIndexer(t, source)
	if err := idx.AddLibrary(10, true, 1); err != nil {
		t.Fatal(err)
	}

	if err := idx.EnrichGoodReads(10); err != nil {
		t.Fatal(err)
	}

	doc, err := idx.documentByIndexID(goodReadsTestID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.GoodReadsBookURL != wantGoodReadsBookURL {
		t.Errorf("unexpected book URL %q", doc.GoodReadsBookURL)
	}
	assertNoDocumentAuthorURL(t, idx.documentsIdx)
	if !doc.GoodReadsEnriched {
		t.Error("expected document to be marked as GoodReads-enriched")
	}
	author, err := idx.Author("alice-borchardt", "")
	if err != nil || author.GoodReadsAuthorURL != wantGoodReadsAuthorURL {
		t.Fatalf("expected matching author URL stored, got %+v, %v", author, err)
	}
	other, err := idx.Author("second-author", "")
	if err != nil || other.GoodReadsAuthorURL != "" {
		t.Fatalf("unmatched coauthor must not receive the URL, got %+v, %v", other, err)
	}
	if want := []string{"La Reina Dragon Alice Borchardt"}; !slices.Equal(source.Queries(), want) {
		t.Errorf("expected queries %q, got %q", want, source.Queries())
	}

	if err := idx.EnrichGoodReads(10); err != nil {
		t.Fatal(err)
	}
	if len(source.Queries()) != 1 {
		t.Errorf("expected enriched documents not to be searched again, got queries %q", source.Queries())
	}
}

func TestEnrichGoodReadsMarksDocumentsWithoutMatch(t *testing.T) {
	for name, source := range map[string]*fakeGoodReadsSource{
		"no results":        {},
		"non-transient err": {err: errors.New("biblioreads: request failed with status code 504")},
		"wrong title":       {results: []biblioreads.SearchResult{{Title: "Another book", Author: "Alice Borchardt", BookURL: "/book/show/1"}}},
		"wrong author":      {results: []biblioreads.SearchResult{{Title: "La Reina Dragon", Author: "Another writer", BookURL: "/book/show/1"}}},
	} {
		t.Run(name, func(t *testing.T) {
			idx := newGoodReadsTestIndexer(t, source)
			if err := idx.AddLibrary(10, true, 1); err != nil {
				t.Fatal(err)
			}

			if err := idx.EnrichGoodReads(10); err != nil {
				t.Fatal(err)
			}
			doc, err := idx.documentByIndexID(goodReadsTestID)
			if err != nil {
				t.Fatal(err)
			}
			if !doc.GoodReadsEnriched || doc.GoodReadsBookURL != "" {
				t.Errorf("expected document enriched with no URLs, got %+v", doc)
			}
			if want := []string{"La Reina Dragon Alice Borchardt", "La Reina Dragon"}; !slices.Equal(source.Queries(), want) {
				t.Errorf("expected both searches before giving up, got %q", source.Queries())
			}
		})
	}
}

func TestEnrichGoodReadsStoresSecondAuthorURLAndPreservesItOnEdits(t *testing.T) {
	const authorURL = "/author/show/2"
	source := &fakeGoodReadsSource{results: []biblioreads.SearchResult{
		{Title: "La Reina Dragon", Author: "Second Author", BookURL: wantGoodReadsBookURL, AuthorURL: authorURL},
	}}
	idx := newGoodReadsTestIndexer(t, source)
	if err := idx.AddLibrary(10, true, 1); err != nil {
		t.Fatal(err)
	}
	stale, err := idx.Author("second-author", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.EnrichGoodReads(10); err != nil {
		t.Fatal(err)
	}
	author, err := idx.Author("second-author", "")
	if err != nil || author.GoodReadsAuthorURL != authorURL {
		t.Fatalf("expected second author URL stored, got %+v, %v", author, err)
	}
	first, err := idx.Author("alice-borchardt", "")
	if err != nil || first.GoodReadsAuthorURL != "" {
		t.Fatalf("first author must not receive coauthor URL, got %+v, %v", first, err)
	}
	stale.DataSourceID = "Q2"
	stale.RetrievedOn = time.Now().UTC()
	if err := idx.IndexAuthor(stale); err != nil {
		t.Fatal(err)
	}
	author, err = idx.Author("second-author", "")
	if err != nil || author.GoodReadsAuthorURL != authorURL || author.DataSourceID != "Q2" || author.DocumentCount != 1 {
		t.Fatalf("stale author edit lost URL or metadata: %+v, %v", author, err)
	}
	if err := idx.incrementAuthorCount(author.Name, author.Slug); err != nil {
		t.Fatal(err)
	}
	author, err = idx.Author("second-author", "")
	if err != nil || author.GoodReadsAuthorURL != authorURL || author.DocumentCount != 2 {
		t.Fatalf("incrementing document count lost author URL: %+v, %v", author, err)
	}
}

func TestEnrichGoodReadsLeavesDocumentsForLaterWhenUnavailable(t *testing.T) {
	source := &fakeGoodReadsSource{err: biblioreads.ErrUnavailable}
	idx := newGoodReadsTestIndexer(t, source)
	if err := idx.AddLibrary(10, true, 1); err != nil {
		t.Fatal(err)
	}

	if err := idx.EnrichGoodReads(10); !errors.Is(err, biblioreads.ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	doc, err := idx.documentByIndexID(goodReadsTestID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.GoodReadsEnriched {
		t.Error("expected document not to be marked as GoodReads-enriched")
	}

	source.mu.Lock()
	source.err = nil
	source.results = goodReadsTestResults
	source.mu.Unlock()
	if err := idx.EnrichGoodReads(10); err != nil {
		t.Fatal(err)
	}
	if doc, _ = idx.documentByIndexID(goodReadsTestID); doc.GoodReadsBookURL != wantGoodReadsBookURL {
		t.Errorf("expected document to be enriched on retry, got %q", doc.GoodReadsBookURL)
	}
}

func TestNewFileSchedulesGoodReadsEnrichment(t *testing.T) {
	source := &fakeGoodReadsSource{results: goodReadsTestResults}
	idx := newGoodReadsTestIndexer(t, source)

	if _, err := idx.NewFile("new.pdf", []byte("")); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		doc, err := idx.documentByIndexID("new.pdf")
		if err != nil {
			t.Fatal(err)
		}
		if doc.GoodReadsEnriched {
			if doc.GoodReadsBookURL != wantGoodReadsBookURL {
				t.Errorf("unexpected book URL %q", doc.GoodReadsBookURL)
			}
			assertNoDocumentAuthorURL(t, idx.documentsIdx)
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for background GoodReads enrichment")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestTextRankWritesPreserveGoodReadsFields guards against TextRank
// enrichment, which writes back documents read before ranking them,
// discarding GoodReads URLs persisted in the meantime, and vice versa.
func TestTextRankWritesPreserveGoodReadsFields(t *testing.T) {
	source := &fakeGoodReadsSource{results: goodReadsTestResults}
	idx := newGoodReadsTestIndexer(t, source)
	if err := idx.AddLibrary(10, true, 1); err != nil {
		t.Fatal(err)
	}
	stale, err := idx.documentByIndexID(goodReadsTestID)
	if err != nil {
		t.Fatal(err)
	}

	if err := idx.EnrichGoodReads(10); err != nil {
		t.Fatal(err)
	}
	stale.TextRankEnriched = false
	if err := idx.StoreGoodReadsBookIDs(stale, wantGoodReadsBookURL, "work-id", 12207444); err != nil {
		t.Fatal(err)
	}
	stale.TextRankWords = []string{"dragon"}
	idx.enrichTextRankAndReindex(stale)

	doc, err := idx.documentByIndexID(goodReadsTestID)
	if err != nil {
		t.Fatal(err)
	}
	if !doc.GoodReadsEnriched || doc.GoodReadsBookURL != wantGoodReadsBookURL {
		t.Errorf("expected GoodReads fields to survive a TextRank write, got %+v", doc)
	}
	if doc.GoodReadsResourceID != "work-id" || doc.GoodReadsLegacyBookID != 12207444 {
		t.Errorf("expected GoodReads identifiers to survive a TextRank write, got %+v", doc)
	}
	if !doc.TextRankEnriched {
		t.Error("expected TextRank write to be persisted")
	}

	// A GoodReads write must keep TextRank fields written after the document was read.
	stale, _ = idx.documentByIndexID(goodReadsTestID)
	stale.GoodReadsEnriched = false
	doc.TextRankWords = []string{"guinevere"}
	idx.documentsMu.Lock()
	if err := idx.documentsIdx.Index(doc.ID, doc); err != nil {
		t.Fatal(err)
	}
	idx.documentsMu.Unlock()
	if _, err := idx.enrichGoodReads(context.Background(), stale, true); err != nil {
		t.Fatal(err)
	}
	if doc, _ = idx.documentByIndexID(goodReadsTestID); !slices.Equal(doc.TextRankWords, []string{"guinevere"}) {
		t.Errorf("expected TextRank words to survive a GoodReads write, got %q", doc.TextRankWords)
	}
}

func TestEnrichGoodReadsWithoutSourceIsNoop(t *testing.T) {
	idx := newGoodReadsTestIndexer(t, nil)
	if err := idx.AddLibrary(10, true, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.NewFile("new.pdf", []byte("")); err != nil {
		t.Fatal(err)
	}
	if err := idx.EnrichGoodReads(10); err != nil {
		t.Fatal(err)
	}
	doc, err := idx.documentByIndexID(goodReadsTestID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.GoodReadsEnriched {
		t.Error("expected document not to be GoodReads-enriched when the source is disabled")
	}
}

func TestEnrichGoodReadsSkipsSearchWhenDocumentHasLink(t *testing.T) {
	source := &fakeGoodReadsSource{results: goodReadsTestResults}
	idx := newGoodReadsTestIndexer(t, source)
	if err := idx.AddLibrary(10, true, 1); err != nil {
		t.Fatal(err)
	}
	doc, err := idx.documentByIndexID(goodReadsTestID)
	if err != nil {
		t.Fatal(err)
	}
	doc.GoodReadsBookURL = "/book/show/1-existing"
	if err := idx.documentsIdx.Index(doc.ID, doc); err != nil {
		t.Fatal(err)
	}

	if err := idx.EnrichGoodReads(10); err != nil {
		t.Fatal(err)
	}

	if len(source.Queries()) != 0 {
		t.Errorf("expected no search for a document with a link, got %q", source.Queries())
	}
	doc, err = idx.documentByIndexID(goodReadsTestID)
	if err != nil {
		t.Fatal(err)
	}
	if !doc.GoodReadsEnriched || doc.GoodReadsBookURL != "/book/show/1-existing" {
		t.Errorf("expected existing link kept and document marked enriched, got %+v", doc)
	}
}

func TestGoodReadsBook(t *testing.T) {
	source := &fakeGoodReadsSource{results: goodReadsTestResults}
	idx := newGoodReadsTestIndexer(t, source)
	if err := idx.AddLibrary(10, true, 1); err != nil {
		t.Fatal(err)
	}
	doc, err := idx.documentByIndexID(goodReadsTestID)
	if err != nil {
		t.Fatal(err)
	}

	got, err := idx.GoodReadsBook(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if got.BookURL != wantGoodReadsBookURL || got.Rating != "3.82" {
		t.Errorf("expected fresh book URL and search rating, got %+v", got)
	}
	stored, err := idx.documentByIndexID(goodReadsTestID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.GoodReadsEnriched || stored.GoodReadsBookURL != wantGoodReadsBookURL {
		t.Errorf("expected the link to be persisted, got %+v", stored)
	}

	if got, _ := idx.GoodReadsBook(context.Background(), stored); got.BookURL != wantGoodReadsBookURL || got.Rating != "" {
		t.Errorf("expected stored URL without a persisted rating, got %+v", got)
	}
	req := bleve.NewSearchRequest(bleve.NewDocIDQuery([]string{stored.ID}))
	req.Fields = []string{"*"}
	result, err := idx.documentsIdx.Search(req)
	if err != nil || len(result.Hits) != 1 {
		t.Fatalf("reading stored Goodreads fields: %v", err)
	}
	for _, field := range []string{"Rating", "rating", "GoodReadsRating"} {
		if _, exists := result.Hits[0].Fields[field]; exists {
			t.Errorf("rating must not be stored in document field %s", field)
		}
	}
	if err := idx.EnrichGoodReads(10); err != nil {
		t.Fatal(err)
	}
	if len(source.Queries()) != 1 {
		t.Errorf("expected a single search, got %q", source.Queries())
	}
}

func TestGoodReadsBookReturnsErrorWhenUnavailable(t *testing.T) {
	source := &fakeGoodReadsSource{err: biblioreads.ErrUnavailable}
	idx := newGoodReadsTestIndexer(t, source)
	if err := idx.AddLibrary(10, true, 1); err != nil {
		t.Fatal(err)
	}
	doc, _ := idx.documentByIndexID(goodReadsTestID)
	if _, err := idx.GoodReadsBook(context.Background(), doc); !errors.Is(err, biblioreads.ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if doc, _ = idx.documentByIndexID(goodReadsTestID); doc.GoodReadsEnriched {
		t.Error("expected document to stay unenriched for a later retry")
	}
}

func TestMatchingGoodReadsBook(t *testing.T) {
	correct := biblioreads.SearchResult{
		Title: "El señor de los anillos", Author: "J.R.R. Tolkien",
		BookURL: "/book/show/37910810", AuthorURL: "/author/show/656983.J_R_R_Tolkien",
	}
	parody := biblioreads.SearchResult{
		Title: "El sopor de los anillos", Author: "The Harvard Lampoon",
		BookURL: "/book/show/7297403-el-sopor-de-los-anillos",
	}
	tests := []struct {
		name    string
		title   string
		authors []string
		results []biblioreads.SearchResult
		want    string
	}{
		{name: "correct result after parody", results: []biblioreads.SearchResult{parody, correct}, want: correct.BookURL},
		{name: "parody rejected", results: []biblioreads.SearchResult{parody}},
		{name: "similar title with correct author rejected", results: []biblioreads.SearchResult{{Title: parody.Title, Author: correct.Author, BookURL: parody.BookURL}}},
		{name: "wrong author rejected", results: []biblioreads.SearchResult{{Title: correct.Title, Author: parody.Author, BookURL: parody.BookURL}}},
		{name: "normalization", title: "  EL SEN\u0303OR DE LOS ANILLOS! ", results: []biblioreads.SearchResult{correct}, want: correct.BookURL},
		{name: "series suffix", results: []biblioreads.SearchResult{{Title: correct.Title + " (El señor de los anillos, #1)", Author: correct.Author, BookURL: correct.BookURL}}, want: correct.BookURL},
		{name: "document series suffix", title: correct.Title + " (Middle Earth, #1)", results: []biblioreads.SearchResult{correct}, want: correct.BookURL},
		{name: "omnibus range", results: []biblioreads.SearchResult{{Title: correct.Title + " (#1-#3)", Author: correct.Author, BookURL: correct.BookURL}}, want: correct.BookURL},
		{name: "omnibus range with dash", results: []biblioreads.SearchResult{{Title: correct.Title + " (#1–3)", Author: correct.Author, BookURL: correct.BookURL}}, want: correct.BookURL},
		{name: "document omnibus range", title: correct.Title + " (#1-#3)", results: []biblioreads.SearchResult{correct}, want: correct.BookURL},
		{name: "standalone volume retained", results: []biblioreads.SearchResult{{Title: correct.Title + " (#1)", Author: correct.Author, BookURL: correct.BookURL}}},
		{name: "exact preferred", results: []biblioreads.SearchResult{{Title: correct.Title + " (Middle Earth, #1)", Author: correct.Author, BookURL: "/book/show/2"}, correct}, want: correct.BookURL},
		{name: "subtitle retained", results: []biblioreads.SearchResult{{Title: correct.Title + ": A parody", Author: correct.Author, BookURL: correct.BookURL}}},
		{name: "volume retained", results: []biblioreads.SearchResult{{Title: correct.Title + " 2", Author: correct.Author, BookURL: correct.BookURL}}},
		{name: "arbitrary parenthetical retained", results: []biblioreads.SearchResult{{Title: correct.Title + " (A parody)", Author: correct.Author, BookURL: correct.BookURL}}},
		{name: "missing result author", results: []biblioreads.SearchResult{{Title: correct.Title, BookURL: correct.BookURL}}},
		{name: "missing result title", results: []biblioreads.SearchResult{{Author: correct.Author, BookURL: correct.BookURL}}},
		{name: "missing URL", results: []biblioreads.SearchResult{{Title: correct.Title, Author: correct.Author}}},
		{name: "any document author", authors: []string{"Another Writer", "J. R. R. Tolkien"}, results: []biblioreads.SearchResult{correct}, want: correct.BookURL},
		{name: "no author", authors: []string{}, results: []biblioreads.SearchResult{correct}},
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
			match, ok := matchingGoodReadsBook(Document{Metadata: metadata.Metadata{Title: title, Authors: authors}}, tt.results)
			if match.BookURL != tt.want || ok != (tt.want != "") {
				t.Fatalf("got %q, %v; want %q", match.BookURL, ok, tt.want)
			}
		})
	}
}

func TestGoodReadsTitleFallback(t *testing.T) {
	const title = "El señor de los anillos"
	correct := biblioreads.SearchResult{Title: title, Author: "J.R.R. Tolkien", BookURL: "/book/show/37910810"}
	parody := biblioreads.SearchResult{Title: "El sopor de los anillos", Author: "The Harvard Lampoon", BookURL: "/book/show/7297403"}
	for _, fallbackUnavailable := range []bool{false, true} {
		name := "matched"
		if fallbackUnavailable {
			name = "unavailable"
		}
		t.Run(name, func(t *testing.T) {
			source := &fakeGoodReadsSource{byQuery: map[string][]biblioreads.SearchResult{
				title + " J. R. R. Tolkien": {parody},
				title:                       {parody, correct},
			}}
			idx := newGoodReadsTestIndexer(t, source)
			doc := Document{ID: goodReadsTestID, AuthorsSlugs: []string{"j-r-r-tolkien"}, Metadata: metadata.Metadata{Title: title, Authors: []string{"J. R. R. Tolkien"}}}
			if err := idx.documentsIdx.Index(doc.ID, doc); err != nil {
				t.Fatal(err)
			}
			if fallbackUnavailable {
				source.queryErrors = map[string]error{title: biblioreads.ErrUnavailable}
			}
			got, err := idx.GoodReadsBook(context.Background(), doc)
			if fallbackUnavailable {
				if !errors.Is(err, biblioreads.ErrUnavailable) || got.BookURL != "" {
					t.Fatalf("expected unavailable error, got %+v, %v", got, err)
				}
				stored, readErr := idx.documentByIndexID(doc.ID)
				if readErr != nil || stored.GoodReadsEnriched {
					t.Fatalf("expected document left for retry, got %+v, %v", stored, readErr)
				}
				source.queryErrors = nil
				got, err = idx.GoodReadsBook(context.Background(), stored)
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

func TestGoodReadsMatchesTolkienOmnibusSearchResults(t *testing.T) {
	const title = "El señor de los anillos"
	const wantURL = "/book/show/37910810-el-se-or-de-los-anillos"
	// Titles and ordering from BiblioReads' actual title-only search response.
	results := []biblioreads.SearchResult{
		{Title: "La comunidad del anillo (El señor de los anillos, #1)", Author: "J.R.R. Tolkien", BookURL: "/book/show/222947.La_comunidad_del_anillo"},
		{Title: "Las dos torres (El señor de los anillos, #2)", Author: "J.R.R. Tolkien", BookURL: "/book/show/15280.Las_dos_torres"},
		{Title: "El retorno del rey (El señor de los anillos, #3)", Author: "J.R.R. Tolkien", BookURL: "/book/show/63358.El_retorno_del_rey"},
		{Title: "El señor de los anillos (#1-#3)", Author: "J.R.R. Tolkien", BookURL: wantURL, AuthorURL: "/author/show/656983.J_R_R_Tolkien"},
		{Title: "Apéndices (El Señor de los Anillos, #4)", Author: "J.R.R. Tolkien", BookURL: "/book/show/54694.Ap_ndices"},
	}
	source := &fakeGoodReadsSource{byQuery: map[string][]biblioreads.SearchResult{title: results}}
	idx := newGoodReadsTestIndexer(t, source)
	doc := Document{ID: goodReadsTestID, AuthorsSlugs: []string{"j-r-r-tolkien"}, Metadata: metadata.Metadata{Title: title, Authors: []string{"J. R. R. Tolkien"}}}
	if err := idx.documentsIdx.Index(doc.ID, doc); err != nil {
		t.Fatal(err)
	}
	got, err := idx.GoodReadsBook(context.Background(), doc)
	if err != nil || got.BookURL != wantURL {
		t.Fatalf("got %+v, %v; want %q", got, err, wantURL)
	}
	stored, err := idx.documentByIndexID(doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.GoodReadsEnriched || stored.GoodReadsBookURL != wantURL {
		t.Fatalf("expected persisted omnibus match, got %+v", stored)
	}
	author, err := idx.Author("j-r-r-tolkien", "")
	if err != nil || author.GoodReadsAuthorURL != results[3].AuthorURL {
		t.Fatalf("expected author URL on author record, got %+v, %v", author, err)
	}
	if want := []string{title + " J. R. R. Tolkien", title}; !slices.Equal(source.Queries(), want) {
		t.Fatalf("queries = %q, want %q", source.Queries(), want)
	}
}

func TestForceIndexingRefreshesGoodReadsMatch(t *testing.T) {
	source := &fakeGoodReadsSource{results: goodReadsTestResults}
	idx := newGoodReadsTestIndexer(t, source)
	if err := idx.AddLibrary(10, true, 1); err != nil {
		t.Fatal(err)
	}
	doc, err := idx.documentByIndexID(goodReadsTestID)
	if err != nil {
		t.Fatal(err)
	}
	doc.GoodReadsBookURL = "/book/show/1-wrong"
	doc.GoodReadsResourceID = "wrong-work"
	doc.GoodReadsLegacyBookID = 1
	doc.GoodReadsEnriched = true
	if err := idx.documentsIdx.Index(doc.ID, doc); err != nil {
		t.Fatal(err)
	}
	if err := idx.AddLibrary(10, true, 1); err != nil {
		t.Fatal(err)
	}
	if err := idx.EnrichGoodReads(10); err != nil {
		t.Fatal(err)
	}
	doc, err = idx.documentByIndexID(goodReadsTestID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.GoodReadsBookURL != wantGoodReadsBookURL {
		t.Fatalf("expected refreshed book link, got %q", doc.GoodReadsBookURL)
	}
	if doc.GoodReadsResourceID != "" || doc.GoodReadsLegacyBookID != 0 {
		t.Fatalf("expected old identifiers to be cleared, got %q and %d", doc.GoodReadsResourceID, doc.GoodReadsLegacyBookID)
	}
}

func TestStoreGoodReadsBookIDs(t *testing.T) {
	for _, scenario := range []string{"linked", "newly linked", "changed metadata", "changed match", "deleted", "invalid identifiers"} {
		t.Run(scenario, func(t *testing.T) {
			idx := newGoodReadsTestIndexer(t, nil)
			doc := Document{
				ID:                goodReadsTestID,
				Metadata:          metadata.Metadata{Title: "La Reina Dragon", Authors: []string{"Alice Borchardt"}},
				GoodReadsBookURL:  wantGoodReadsBookURL,
				GoodReadsEnriched: true,
				TextRankWords:     []string{"dragon"},
			}
			stale := doc
			resourceID := "work-id"
			switch scenario {
			case "newly linked":
				stale.GoodReadsBookURL = ""
				stale.GoodReadsEnriched = false
			case "changed metadata":
				doc.Title = "Another title"
			case "changed match":
				doc.GoodReadsBookURL = "/book/show/2-other"
			case "invalid identifiers":
				resourceID = ""
			}
			if scenario != "deleted" {
				if err := idx.documentsIdx.Index(doc.ID, doc); err != nil {
					t.Fatal(err)
				}
			}
			err := idx.StoreGoodReadsBookIDs(stale, wantGoodReadsBookURL, resourceID, 12207444)
			wantError := scenario != "linked" && scenario != "newly linked"
			if (err != nil) != wantError {
				t.Fatalf("StoreGoodReadsBookIDs error = %v, want error %v", err, wantError)
			}
			if scenario == "deleted" {
				if !errors.Is(err, ErrDocumentNotFound) {
					t.Fatalf("expected ErrDocumentNotFound, got %v", err)
				}
				return
			}
			stored, err := idx.documentByIndexID(doc.ID)
			if err != nil {
				t.Fatal(err)
			}
			if wantError {
				if stored.GoodReadsResourceID != "" || stored.GoodReadsLegacyBookID != 0 {
					t.Fatalf("unexpected identifiers persisted: %+v", stored)
				}
				return
			}
			if stored.GoodReadsResourceID != resourceID || stored.GoodReadsLegacyBookID != 12207444 {
				t.Fatalf("identifiers not persisted: %+v", stored)
			}
			if !stored.GoodReadsEnriched || !slices.Equal(stored.TextRankWords, doc.TextRankWords) {
				t.Fatalf("existing enrichment was lost: %+v", stored)
			}
			if err := idx.StoreGoodReadsBookIDs(stale, wantGoodReadsBookURL, resourceID, 12207444); err != nil {
				t.Fatalf("repeated store failed: %v", err)
			}
		})
	}
}
