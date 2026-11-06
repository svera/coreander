package index

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/svera/coreander/v5/internal/metadata"
)

func TestTextRankWritesPreserveProviderRemovals(t *testing.T) {
	for _, keepIndependent := range []bool{false, true} {
		name := "all providers reset"
		if keepIndependent {
			name = "one provider reset"
		}
		t.Run(name, func(t *testing.T) {
			idx, documents := newTestTextRankIndexer(t, Config{})
			t.Cleanup(func() { _ = idx.Close() })
			document := withProviderData(t, Document{
				ID: "book", Slug: "book", Metadata: metadata.Metadata{Title: "Book"},
				DataSourceScoreValue: 3.82,
			}, testProviderData{URL: "/book/show/123", Enriched: true})
			if err := SetDataSourceData(&document, "other", map[string]any{"enriched": true}); err != nil {
				t.Fatal(err)
			}
			if err := IndexDocument(documents, document.ID, document); err != nil {
				t.Fatal(err)
			}
			stale, err := idx.documentByIndexID("book")
			if err != nil {
				t.Fatal(err)
			}
			if err := idx.UpdateDocument("book", func(update *DocumentUpdate) (bool, error) {
				if keepIndependent {
					delete(update.DataSources, "dummy")
				} else {
					update.DataSources = nil
				}
				update.DataSourceScoreValue = 0
				return true, nil
			}); err != nil {
				t.Fatal(err)
			}
			latest, err := idx.documentByIndexID("book")
			if err != nil {
				t.Fatal(err)
			}

			idx.enrichTextRankAndReindex(stale)
			got, err := idx.documentByIndexID("book")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.DataSources, latest.DataSources) || got.DataSourceScoreValue != 0 {
				t.Fatalf("stale TextRank write restored removed provider data: %+v", got)
			}
			if !got.TextRankEnriched {
				t.Fatal("TextRank result must still be persisted")
			}
			pending, total, err := idx.documentsNeedingProvider("dummy", 10, nil)
			if err != nil || total != 1 || len(pending) != 1 || pending[0].ID != "book" {
				t.Fatalf("reset provider lookup must remain pending: %+v, %d, %v", pending, total, err)
			}
		})
	}
}

// TestTextRankWritesPreserveProviderFields guards against TextRank
// enrichment, which writes back documents read before ranking them,
// discarding provider data persisted in the meantime, and vice versa.
func TestTextRankWritesPreserveProviderFields(t *testing.T) {
	source := &testDocumentEnricher{source: "dummy"}
	idx, documents := newTestTextRankIndexer(t, Config{DocumentEnrichers: []DocumentEnricher{source}})
	t.Cleanup(func() { _ = idx.Close() })
	if err := IndexDocument(documents, "book", Document{ID: "book", Metadata: metadata.Metadata{Title: "Book"}}); err != nil {
		t.Fatal(err)
	}
	stale, err := idx.documentByIndexID("book")
	if err != nil {
		t.Fatal(err)
	}

	if err := idx.EnrichDocuments(10); err != nil {
		t.Fatal(err)
	}
	stale.TextRankEnriched = false
	want := testProviderData{
		URL: "provider-reference", Enriched: true, RatingCount: "2,285",
		ResourceID: "work-id", LegacyBookID: 12207444,
	}
	if err := idx.UpdateDocument(stale.ID, func(update *DocumentUpdate) (bool, error) {
		update.DataSourceScoreValue = 3.82
		return true, SetDataSourceData(&update.Document, source.Source(), want)
	}); err != nil {
		t.Fatal(err)
	}
	stale.TextRankWords = []string{"dragon"}
	idx.enrichTextRankAndReindex(stale)

	doc, err := idx.documentByIndexID("book")
	if err != nil {
		t.Fatal(err)
	}
	if data := mustProviderData(t, doc); data != want || doc.DataSourceScoreValue != 3.82 {
		t.Errorf("expected provider fields and identifiers to survive a TextRank write, got %+v", doc)
	}
	if !doc.TextRankEnriched {
		t.Error("expected TextRank write to be persisted")
	}

	// A Provider write must keep TextRank fields written after the document was read.
	stale, _ = idx.documentByIndexID("book")
	if err := SetDataSourceData(&stale, source.Source(), testProviderData{Enriched: false}); err != nil {
		t.Fatal(err)
	}
	doc.TextRankWords = []string{"guinevere"}
	idx.documentsMu.Lock()
	if err := IndexDocument(idx.documentsIdx, doc.ID, doc); err != nil {
		t.Fatal(err)
	}
	idx.documentsMu.Unlock()
	if err := source.Enrich(context.Background(), idx, stale); err != nil {
		t.Fatal(err)
	}
	if doc, _ = idx.documentByIndexID("book"); !slices.Equal(doc.TextRankWords, []string{"guinevere"}) {
		t.Errorf("expected TextRank words to survive a Provider write, got %q", doc.TextRankWords)
	}
}
