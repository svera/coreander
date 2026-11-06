package index

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/svera/coreander/v5/internal/metadata"
)

func TestDocumentDataSourcesRoundTrip(t *testing.T) {
	idx, _ := newTestTextRankIndexer(t, Config{})
	t.Cleanup(func() { _ = idx.Close() })
	for _, language := range []string{"es", ""} {
		document := withProviderData(t, Document{
			ID: "book-" + language, Slug: "book-" + language,
			Metadata:             metadata.Metadata{Title: "A book", Language: language, Format: "EPUB"},
			DataSourceScoreValue: 4.25,
		}, testProviderData{URL: "/book/show/123", Enriched: true})
		other := map[string]any{
			"id": "opaqueprovideridentifier", "nested": map[string]any{"items": []any{"privatecatalogtoken", float64(42)}},
		}
		if err := SetDataSourceData(&document, "other", other); err != nil {
			t.Fatal(err)
		}
		if err := IndexDocument(idx.documentsIdx, document.ID, document); err != nil {
			t.Fatal(err)
		}
		got, err := idx.documentByIndexID(document.ID)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := DataSourceData[map[string]any](got, "other")
		if err != nil || !reflect.DeepEqual(decoded, other) {
			t.Fatalf("provider payload changed: %+v, %v", decoded, err)
		}
		if mustProviderData(t, got) != mustProviderData(t, document) || got.DataSourceScoreValue != 4.25 {
			t.Fatalf("provider payload or score changed: %+v", got)
		}
		for _, term := range []string{"opaqueprovideridentifier", "privatecatalogtoken", "dummy", "other"} {
			request := bleve.NewSearchRequest(bleve.NewMatchQuery(term))
			result, err := idx.documentsIdx.Search(request)
			if err != nil || result.Total != 0 {
				t.Fatalf("opaque data affected keyword search for %q: %+v, %v", term, result, err)
			}
		}
	}
	if _, total, err := idx.documentsNeedingProvider("dummy", 10, nil); err != nil || total != 0 {
		t.Fatalf("enriched payloads were queued again: %d, %v", total, err)
	}
	pending := Document{ID: "other-only", Slug: "other-only", Metadata: metadata.Metadata{Title: "Pending book"}}
	if err := SetDataSourceData(&pending, "other", map[string]any{"enriched": true}); err != nil {
		t.Fatal(err)
	}
	if err := IndexDocument(idx.documentsIdx, pending.ID, pending); err != nil {
		t.Fatal(err)
	}
	if documents, total, err := idx.documentsNeedingProvider("dummy", 10, nil); err != nil || total != 1 || documents[0].ID != pending.ID {
		t.Fatalf("another provider marked provider as enriched: %+v, %d, %v", documents, total, err)
	}
}

func TestDocumentDataSourcesRejectInvalidData(t *testing.T) {
	idx, _ := newTestTextRankIndexer(t, Config{})
	t.Cleanup(func() { _ = idx.Close() })
	document := Document{ID: "book", Slug: "book", Metadata: metadata.Metadata{Title: "Book"}}
	for _, invalid := range []any{nil, "not an object", []int{1}, make(chan int)} {
		if err := SetDataSourceData(&document, "invalid", invalid); err == nil {
			t.Fatalf("accepted non-object provider payload: %#v", invalid)
		}
		if document.DataSources != nil {
			t.Fatal("failed update mutated document data")
		}
	}
	for _, raw := range []string{`{"url":`, `null`, `[]`} {
		document.DataSources = map[string]json.RawMessage{"dummy": json.RawMessage(raw)}
		if err := IndexDocument(idx.documentsIdx, document.ID, document); err == nil {
			t.Fatalf("indexed invalid payload: %s", raw)
		}
	}
}

func TestDocumentDataSourcesSurviveProviderAndStaleWrites(t *testing.T) {
	idx, _ := newTestTextRankIndexer(t, Config{})
	t.Cleanup(func() { _ = idx.Close() })
	if err := IndexDocument(idx.documentsIdx, "book", Document{ID: "book", Slug: "book", Metadata: metadata.Metadata{Title: "Book"}}); err != nil {
		t.Fatal(err)
	}
	stale, err := idx.documentByIndexID("book")
	if err != nil {
		t.Fatal(err)
	}
	current := stale
	other := map[string]any{"id": "external-id", "nested": map[string]any{"count": float64(3)}}
	if err := SetDataSourceData(&current, "other", other); err != nil {
		t.Fatal(err)
	}
	if _, exists := stale.DataSources["other"]; exists {
		t.Fatal("updating a provider mutated another document's map")
	}
	if err := IndexDocument(idx.documentsIdx, current.ID, current); err != nil {
		t.Fatal(err)
	}
	if err := idx.StoreDocumentData(stale.ID, "dummy", testProviderData{URL: "/book/show/123", Enriched: true}); err != nil {
		t.Fatal(err)
	}
	if err := idx.UpdateDocument(stale.ID, func(update *DocumentUpdate) (bool, error) {
		update.DataSourceScoreValue = 4.5
		return true, SetDataSourceData(&update.Document, "dummy", testProviderData{URL: "/book/show/123", Enriched: true, RatingCount: "100", ResourceID: "work", LegacyBookID: 123})
	}); err != nil {
		t.Fatal(err)
	}
	if err := idx.StoreDocumentData(stale.ID, "dummy", testProviderData{URL: "/book/show/456", Enriched: true}); err != nil {
		t.Fatal(err)
	}
	stale.TextRankWords = []string{"updated"}
	documents := []Document{stale}
	idx.documentsMu.Lock()
	err = idx.preserveDataSourcesLocked(documents)
	if err == nil {
		err = IndexDocument(idx.documentsIdx, stale.ID, documents[0])
	}
	idx.documentsMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	got, err := idx.documentByIndexID(stale.ID)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DataSourceData[map[string]any](got, "other")
	if err != nil || !reflect.DeepEqual(decoded, other) {
		t.Fatalf("another provider's payload was lost: %+v, %v", decoded, err)
	}
	if data := mustProviderData(t, got); data != (testProviderData{URL: "/book/show/456", Enriched: true}) {
		t.Fatalf("stale write restored old provider state: %+v", data)
	}
}

type testProviderData struct {
	URL          string `json:"url"`
	Enriched     bool   `json:"enriched"`
	RatingCount  string `json:"ratingCount,omitempty"`
	ResourceID   string `json:"resourceId,omitempty"`
	LegacyBookID int    `json:"legacyBookId,omitempty"`
}

func mustProviderData(t *testing.T, document Document) testProviderData {
	t.Helper()
	data, err := DataSourceData[testProviderData](document, "dummy")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func withProviderData(t *testing.T, document Document, data testProviderData) Document {
	t.Helper()
	if err := SetDataSourceData(&document, "dummy", data); err != nil {
		t.Fatal(err)
	}
	return document
}

type documentUpdateBaseIndex interface {
	bleve.Index
}

type documentUpdateCountingIndex struct {
	documentUpdateBaseIndex
	writes int
}

func (i *documentUpdateCountingIndex) Index(id string, data any) error {
	i.writes++
	return i.documentUpdateBaseIndex.Index(id, data)
}

func newDocumentUpdateTestIndexer(t *testing.T) (*BleveIndexer, *documentUpdateCountingIndex, *documentUpdateCountingIndex) {
	t.Helper()
	documents, err := bleve.NewMemOnly(CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}
	authors, err := bleve.NewMemOnly(CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}
	documentWrites := &documentUpdateCountingIndex{documentUpdateBaseIndex: documents}
	authorWrites := &documentUpdateCountingIndex{documentUpdateBaseIndex: authors}
	idx := NewBleve(documentWrites, authorWrites, nil, "", nil, Config{})
	t.Cleanup(func() { _ = idx.Close() })
	return idx, documentWrites, authorWrites
}

func TestUpdateDocumentSkipsUnchangedAndFailedUpdates(t *testing.T) {
	failure := errors.New("provider failed")
	for _, scenario := range []struct {
		name    string
		changed bool
		err     error
	}{
		{name: "unchanged"},
		{name: "failed", changed: true, err: failure},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			idx, documents, authors := newDocumentUpdateTestIndexer(t)
			document := Document{ID: "book", Slug: "book", Metadata: metadata.Metadata{Title: "Original"}}
			if err := IndexDocument(documents, document.ID, document); err != nil {
				t.Fatal(err)
			}
			before, err := idx.DocumentByID(document.ID)
			if err != nil {
				t.Fatal(err)
			}
			documents.writes = 0
			called := false
			err = idx.UpdateDocument(document.ID, func(update *DocumentUpdate) (bool, error) {
				called = true
				update.Title = "Changed"
				update.AuthorLinks = []AuthorLink{{Slug: "writer", Name: "Writer", Source: "dummy", URL: "reference"}}
				if err := SetDataSourceData(&update.Document, "dummy", testProviderData{Enriched: true}); err != nil {
					t.Fatal(err)
				}
				return scenario.changed, scenario.err
			})
			if !called || !errors.Is(err, scenario.err) {
				t.Fatalf("callback called = %v, error = %v, want %v", called, err, scenario.err)
			}
			if documents.writes != 0 || authors.writes != 0 {
				t.Fatalf("skipped update wrote documents/authors: %d/%d", documents.writes, authors.writes)
			}
			after, err := idx.DocumentByID(document.ID)
			if err != nil || !reflect.DeepEqual(after, before) {
				t.Fatalf("skipped update changed document: %+v, %v", after, err)
			}
		})
	}
}

func TestUpdateDocumentMissingDocumentDoesNotCallCallback(t *testing.T) {
	idx, documents, authors := newDocumentUpdateTestIndexer(t)
	called := false
	err := idx.UpdateDocument("missing", func(*DocumentUpdate) (bool, error) {
		called = true
		return true, nil
	})
	if !errors.Is(err, ErrDocumentNotFound) || called || documents.writes != 0 || authors.writes != 0 {
		t.Fatalf("missing-document update: error %v, callback %v, writes %d/%d", err, called, documents.writes, authors.writes)
	}
}

func TestUpdateDocumentRejectsMalformedProviderData(t *testing.T) {
	for _, raw := range []string{`{"url":`, `null`, `[]`} {
		t.Run(raw, func(t *testing.T) {
			idx, documents, _ := newDocumentUpdateTestIndexer(t)
			if err := IndexDocument(documents, "book", Document{ID: "book", Metadata: metadata.Metadata{Title: "Original"}}); err != nil {
				t.Fatal(err)
			}
			documents.writes = 0
			err := idx.UpdateDocument("book", func(update *DocumentUpdate) (bool, error) {
				update.Title = "Invalid update"
				update.DataSources = map[string]json.RawMessage{"dummy": json.RawMessage(raw)}
				return true, nil
			})
			if err == nil || documents.writes != 0 {
				t.Fatalf("malformed data accepted: error %v, writes %d", err, documents.writes)
			}
			stored, err := idx.DocumentByID("book")
			if err != nil || stored.Title != "Original" || len(stored.DataSources) != 0 {
				t.Fatalf("malformed update changed document: %+v, %v", stored, err)
			}
		})
	}
}

func TestUpdateDocumentStoresAuthorLinksFirstWins(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "new author"
		if existing {
			name = "existing author"
		}
		t.Run(name, func(t *testing.T) {
			idx, documents, _ := newDocumentUpdateTestIndexer(t)
			for _, document := range []Document{
				{ID: "book", AuthorsSlugs: []string{"writer"}},
				{ID: "second", AuthorsSlugs: []string{"writer"}},
				{ID: "illustrated", IllustratorsSlugs: []string{"writer"}},
				{ID: "unrelated", AuthorsSlugs: []string{"other"}},
			} {
				if err := IndexDocument(documents, document.ID, document); err != nil {
					t.Fatal(err)
				}
			}
			if existing {
				if err := idx.IndexAuthor(Author{
					Name: "Existing Writer", Slug: "writer", DocumentCount: 42, DataSourceID: "external-id",
					DataSourceLinks: map[string]string{"other": "other-reference"},
				}); err != nil {
					t.Fatal(err)
				}
			}
			for _, url := range []string{"first-reference", "second-reference"} {
				if err := idx.UpdateDocument("book", func(update *DocumentUpdate) (bool, error) {
					update.Title = url
					update.AuthorLinks = []AuthorLink{{Slug: "writer", Name: "Writer", Source: "dummy", URL: url}}
					return true, nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			author, err := idx.Author("writer", "")
			if err != nil || author.DataSourceLinks["dummy"] != "first-reference" {
				t.Fatalf("first author link lost: %+v, %v", author, err)
			}
			if existing {
				if author.Name != "Existing Writer" || author.DocumentCount != 42 ||
					author.DataSourceID != "external-id" || author.DataSourceLinks["other"] != "other-reference" {
					t.Fatalf("existing author metadata changed: %+v", author)
				}
			} else if author.Name != "Writer" || author.DocumentCount != 3 {
				t.Fatalf("new author name or author/illustrator count incorrect: %+v", author)
			}
			document, err := idx.DocumentByID("book")
			if err != nil || document.Title != "second-reference" {
				t.Fatalf("author link prevented document update: %+v, %v", document, err)
			}
		})
	}
}
