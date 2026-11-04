package index

import (
	"encoding/json"
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
	if _, total, err := idx.documentsNeedingProvider("dummy", 10); err != nil || total != 0 {
		t.Fatalf("enriched payloads were queued again: %d, %v", total, err)
	}
	pending := Document{ID: "other-only", Slug: "other-only", Metadata: metadata.Metadata{Title: "Pending book"}}
	if err := SetDataSourceData(&pending, "other", map[string]any{"enriched": true}); err != nil {
		t.Fatal(err)
	}
	if err := IndexDocument(idx.documentsIdx, pending.ID, pending); err != nil {
		t.Fatal(err)
	}
	if documents, total, err := idx.documentsNeedingProvider("dummy", 10); err != nil || total != 1 || documents[0].ID != pending.ID {
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
