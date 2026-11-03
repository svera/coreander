package index

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
	"github.com/svera/coreander/v5/internal/metadata"
)

func TestDocumentDataSourcesRoundTrip(t *testing.T) {
	idx := newGoodReadsTestIndexer(t, nil)
	for _, language := range []string{"es", ""} {
		document := withGoodReadsData(t, Document{
			ID: "book-" + language, Slug: "book-" + language,
			Metadata:             metadata.Metadata{Title: "A book", Language: language, Format: "EPUB"},
			DataSourceScoreValue: 4.25,
		}, biblioreads.DocumentData{URL: "/book/show/123", Enriched: true})
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
		if mustGoodReadsData(t, got) != mustGoodReadsData(t, document) || got.DataSourceScoreValue != 4.25 {
			t.Fatalf("Goodreads payload or score changed: %+v", got)
		}
		for _, term := range []string{"opaqueprovideridentifier", "privatecatalogtoken", "goodreads", "other"} {
			request := bleve.NewSearchRequest(bleve.NewMatchQuery(term))
			result, err := idx.documentsIdx.Search(request)
			if err != nil || result.Total != 0 {
				t.Fatalf("opaque data affected keyword search for %q: %+v, %v", term, result, err)
			}
		}
	}
	if _, total, err := idx.documentsNeedingGoodReads(10); err != nil || total != 0 {
		t.Fatalf("enriched payloads were queued again: %d, %v", total, err)
	}
	pending := Document{ID: "other-only", Slug: "other-only", Metadata: metadata.Metadata{Title: "Pending book"}}
	if err := SetDataSourceData(&pending, "other", map[string]any{"enriched": true}); err != nil {
		t.Fatal(err)
	}
	if err := IndexDocument(idx.documentsIdx, pending.ID, pending); err != nil {
		t.Fatal(err)
	}
	if documents, total, err := idx.documentsNeedingGoodReads(10); err != nil || total != 1 || documents[0].ID != pending.ID {
		t.Fatalf("another provider marked Goodreads as enriched: %+v, %d, %v", documents, total, err)
	}
}

func TestDocumentDataSourcesRejectInvalidData(t *testing.T) {
	idx := newGoodReadsTestIndexer(t, nil)
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
		document.DataSources = map[string]json.RawMessage{biblioreads.Source: json.RawMessage(raw)}
		if err := IndexDocument(idx.documentsIdx, document.ID, document); err == nil {
			t.Fatalf("indexed invalid payload: %s", raw)
		}
	}
	document.DataSources = map[string]json.RawMessage{biblioreads.Source: json.RawMessage(`{"url":42}`)}
	if _, err := idx.GoodReadsBook(context.Background(), document); err == nil {
		t.Fatal("Goodreads accepted an incorrectly typed URL")
	}
}

func TestDocumentDataSourcesSurviveProviderAndStaleWrites(t *testing.T) {
	idx := newGoodReadsTestIndexer(t, &fakeGoodReadsSource{results: goodReadsTestResults})
	if err := idx.AddLibrary(10, true, 1); err != nil {
		t.Fatal(err)
	}
	stale, err := idx.documentByIndexID(goodReadsTestID)
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
	if _, err := idx.enrichGoodReads(context.Background(), stale); err != nil {
		t.Fatal(err)
	}
	if err := idx.StoreGoodReadsDetails(stale, wantDataSourceLink, biblioreads.BookResponse{
		Rating: "4.5", RatingCount: "100", ResourceID: "work", LegacyBookID: 123,
	}); err != nil {
		t.Fatal(err)
	}
	if err := idx.UpdateDataSourceLink(current.ID, "/book/show/456"); err != nil {
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
	if data := mustGoodReadsData(t, got); data != (biblioreads.DocumentData{URL: "/book/show/456", Enriched: true}) {
		t.Fatalf("stale write restored old Goodreads state: %+v", data)
	}
}
