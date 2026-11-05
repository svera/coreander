package index

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/svera/coreander/v5/internal/metadata"
)

type testDocumentEnricher struct {
	source    string
	calls     atomic.Int32
	failure   error
	failureID string
}

type blockingDocumentEnricher struct {
	started chan struct{}
	release chan struct{}
}

func (*blockingDocumentEnricher) Source() string { return "blocking" }

func (*blockingDocumentEnricher) Name() string { return "Blocking" }

func (*blockingDocumentEnricher) NeedsEnrichment(Document) (bool, error) {
	return true, nil
}

func (p *blockingDocumentEnricher) Enrich(ctx context.Context, _ *BleveIndexer, _ Document) error {
	p.started <- struct{}{}
	select {
	case <-p.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestDocumentEnrichmentUsesBoundedConcurrency(t *testing.T) {
	const documentCount = 2 * documentEnrichWorkers
	provider := &blockingDocumentEnricher{
		started: make(chan struct{}, documentCount),
		release: make(chan struct{}),
	}
	idx, _ := newTestTextRankIndexer(t, Config{})
	documents := make([]Document, documentCount)
	done := make(chan map[string]error, 1)
	go func() { done <- idx.enrichProviderBatch(provider, &progressTracker{}, documents) }()
	defer func() {
		close(provider.release)
		if failures := <-done; len(failures) != 0 {
			t.Errorf("enrichment failed: %v", failures)
		}
	}()
	for range documentEnrichWorkers {
		select {
		case <-provider.started:
		case <-time.After(time.Second):
			t.Fatal("expected concurrent provider lookups")
		}
	}
	select {
	case <-provider.started:
		t.Fatal("enrichment exceeded its concurrency bound")
	case <-time.After(30 * time.Millisecond):
	}
}

func TestBackgroundProgressTracksPersistedProviderLookups(t *testing.T) {
	first := &testDocumentEnricher{source: "first", failure: errors.New("unavailable")}
	second := &testDocumentEnricher{source: "second"}
	idx, _ := newTestTextRankIndexer(t, Config{DocumentEnrichers: []DocumentEnricher{first, second}})
	t.Cleanup(func() { _ = idx.Close() })
	for _, id := range []string{"matched", "unmatched", "pending"} {
		if err := IndexDocument(idx.documentsIdx, id, Document{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	// A completed lookup without a score is still processed.
	for _, id := range []string{"matched", "unmatched"} {
		if err := idx.StoreDocumentData(id, "first", map[string]any{"enriched": true}); err != nil {
			t.Fatal(err)
		}
	}
	idx.indexProgress.begin(3)
	idx.indexProgress.record()
	idx.textRankEnrichProgress.begin(3)
	idx.textRankEnrichProgress.record()
	idx.authorEnrichProgress.begin(3)
	idx.authorEnrichProgress.record()
	idx.pruneProgress.begin(3)
	idx.pruneProgress.record()
	progress, err := idx.BackgroundProgress()
	if err != nil {
		t.Fatal(err)
	}
	if len(progress) != 6 || progress[0].Kind != ProgressDocuments || progress[1].Kind != ProgressAuthors ||
		progress[2].Kind != ProgressTextRank || progress[3].Kind != ProgressPruning ||
		progress[4].Name != "first" || progress[4].Percentage != 66 ||
		progress[5].Name != "second" || progress[5].Percentage != 0 {
		t.Fatalf("incorrect simultaneous progress: %+v", progress)
	}
	if progress[4].RemainingTime != 0 {
		t.Fatalf("estimated time without an active lookup run: %v", progress[4].RemainingTime)
	}
	// One lookup in this run took a minute, so the remaining one should take about a minute.
	run := &idx.documentEnrichProgress[0]
	run.begin(1)
	run.startNanos.Store(time.Now().Add(-time.Minute).UnixNano())
	run.record()
	progress, err = idx.BackgroundProgress()
	if err != nil || progress[4].RemainingTime < time.Minute || progress[4].RemainingTime > time.Minute+time.Second {
		t.Fatalf("incorrect review lookup estimate: %+v, %v", progress, err)
	}
	run.end()
	idx.indexProgress.end()
	idx.textRankEnrichProgress.end()
	idx.authorEnrichProgress.end()
	idx.pruneProgress.end()
	if err := idx.EnrichDocuments(1); err == nil {
		t.Fatal("expected unavailable provider to fail")
	}
	progress, err = idx.BackgroundProgress()
	if err != nil || len(progress) != 1 || progress[0].Name != "first" || progress[0].Percentage != 66 {
		t.Fatalf("failed lookups must remain visible; successful providers must disappear: %+v, %v", progress, err)
	}
	// Progress is reconstructed from stored data, not a running worker counter.
	resumed := &BleveIndexer{documentsIdx: idx.documentsIdx, documentEnrichers: []DocumentEnricher{first}}
	progress, err = resumed.BackgroundProgress()
	if err != nil || len(progress) != 1 || progress[0].Percentage != 66 {
		t.Fatalf("persisted progress lost on restart: %+v, %v", progress, err)
	}
	if err := idx.StoreDocumentData("pending", "first", map[string]any{"enriched": true}); err != nil {
		t.Fatal(err)
	}
	progress, err = idx.BackgroundProgress()
	if err != nil || len(progress) != 0 {
		t.Fatalf("completed lookups still reported as pending: %+v, %v", progress, err)
	}
}

func TestBackgroundProgressNeverRoundsPendingLookupsToComplete(t *testing.T) {
	idx, _ := newTestTextRankIndexer(t, Config{DocumentEnrichers: []DocumentEnricher{&testDocumentEnricher{source: "test"}}})
	t.Cleanup(func() { _ = idx.Close() })
	for i := range 201 {
		document := Document{ID: fmt.Sprintf("book-%d", i)}
		if i < 200 {
			if err := SetDataSourceData(&document, "test", map[string]any{"enriched": true}); err != nil {
				t.Fatal(err)
			}
		}
		if err := IndexDocument(idx.documentsIdx, document.ID, document); err != nil {
			t.Fatal(err)
		}
	}
	progress, err := idx.BackgroundProgress()
	if err != nil || len(progress) != 1 || progress[0].Percentage != 99 {
		t.Fatalf("unfinished lookups must not display 100%%: %+v, %v", progress, err)
	}
}

func TestBackgroundProgressWithoutPendingProviders(t *testing.T) {
	for _, config := range []Config{{}, {DocumentEnrichers: []DocumentEnricher{&testDocumentEnricher{source: "test"}}}} {
		idx, _ := newTestTextRankIndexer(t, config)
		t.Cleanup(func() { _ = idx.Close() })
		progress, err := idx.BackgroundProgress()
		if err != nil || len(progress) != 0 {
			t.Fatalf("empty or disabled provider should have no progress: %+v, %v", progress, err)
		}
	}
}

func (p *testDocumentEnricher) Source() string { return p.source }

func (p *testDocumentEnricher) Name() string { return p.source }

func (p *testDocumentEnricher) NeedsEnrichment(document Document) (bool, error) {
	data, err := DataSourceData[struct{ Enriched bool }](document, p.source)
	return !data.Enriched, err
}

func (p *testDocumentEnricher) Enrich(_ context.Context, idx *BleveIndexer, document Document) error {
	p.calls.Add(1)
	if p.failure != nil && (p.failureID == "" || p.failureID == document.ID) {
		return p.failure
	}
	return idx.StoreDocumentData(document.ID, p.source, map[string]any{
		"enriched": true, "reference": p.source + "-reference",
	})
}

func TestDocumentEnrichmentContinuesAfterDocumentFailure(t *testing.T) {
	unavailable := errors.New("document unavailable")
	provider := &testDocumentEnricher{source: "first", failure: unavailable, failureID: "book-2"}
	idx, _ := newTestTextRankIndexer(t, Config{DocumentEnrichers: []DocumentEnricher{provider}})
	t.Cleanup(func() { _ = idx.Close() })
	for _, id := range []string{"book-1", "book-2", "book-3"} {
		if err := IndexDocument(idx.documentsIdx, id, Document{ID: id}); err != nil {
			t.Fatal(err)
		}
	}

	if err := idx.EnrichDocuments(1); !errors.Is(err, unavailable) {
		t.Fatalf("expected the individual lookup error, got %v", err)
	}
	if provider.calls.Load() != 3 {
		t.Fatalf("expected all three documents to be attempted, got %d", provider.calls.Load())
	}
	for _, id := range []string{"book-1", "book-3"} {
		document, err := idx.documentByIndexID(id)
		if err != nil {
			t.Fatal(err)
		}
		data, err := DataSourceData[map[string]any](document, "first")
		if err != nil || data["enriched"] != true {
			t.Errorf("%s was not enriched despite another document failing: %+v, %v", id, data, err)
		}
	}
	pending, _, err := idx.documentsNeedingProvider("first", 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != "book-2" {
		t.Fatalf("failed document should remain pending for a later retry: %+v", pending)
	}
}

func TestDocumentEnrichmentSupportsIndependentProviders(t *testing.T) {
	first := &testDocumentEnricher{source: "first"}
	second := &testDocumentEnricher{source: "second"}
	idx, _ := newTestTextRankIndexer(t, Config{DocumentEnrichers: []DocumentEnricher{first, second}})
	document := Document{ID: "book", Slug: "book", Metadata: metadata.Metadata{Title: "Book"}}
	if err := IndexDocument(idx.documentsIdx, document.ID, document); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := idx.EnrichDocuments(1); err != nil {
			t.Fatal(err)
		}
	}
	stored, err := idx.documentByIndexID(document.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, provider := range []*testDocumentEnricher{first, second} {
		if provider.calls.Load() != 1 {
			t.Fatalf("%s enriched an already processed document", provider.source)
		}
		data, err := DataSourceData[map[string]any](stored, provider.source)
		if err != nil || data["reference"] != provider.source+"-reference" {
			t.Fatalf("provider payload lost: %+v, %v", data, err)
		}
	}
}

func TestDocumentEnrichmentContinuesAfterProviderFailure(t *testing.T) {
	unavailable := errors.New("provider unavailable")
	first := &testDocumentEnricher{source: "first", failure: unavailable}
	second := &testDocumentEnricher{source: "second"}
	idx, _ := newTestTextRankIndexer(t, Config{DocumentEnrichers: []DocumentEnricher{first, second}})
	if err := IndexDocument(idx.documentsIdx, "book", Document{ID: "book", Metadata: metadata.Metadata{Title: "Book"}}); err != nil {
		t.Fatal(err)
	}
	if err := idx.EnrichDocuments(1); !errors.Is(err, unavailable) {
		t.Fatalf("expected provider error, got %v", err)
	}
	if second.calls.Load() != 1 {
		t.Fatal("one provider failure prevented another provider from running")
	}
	first.failure = nil
	if err := idx.EnrichDocuments(1); err != nil {
		t.Fatal(err)
	}
	if first.calls.Load() != 2 || second.calls.Load() != 1 {
		t.Fatal("retry did not resume only the failed provider")
	}
}

func TestDocumentEnrichmentQueueUsesConfiguredProviders(t *testing.T) {
	first := &testDocumentEnricher{source: "first"}
	second := &testDocumentEnricher{source: "second"}
	idx, _ := newTestTextRankIndexer(t, Config{DocumentEnrichers: []DocumentEnricher{first, second}})
	document := Document{ID: "book", Slug: "book", Metadata: metadata.Metadata{Title: "Book"}}
	if err := IndexDocument(idx.documentsIdx, document.ID, document); err != nil {
		t.Fatal(err)
	}
	idx.scheduleDocumentEnrichment(document.ID)
	deadline := time.Now().Add(2 * time.Second)
	for {
		stored, err := idx.documentByIndexID(document.ID)
		if err != nil {
			t.Fatal(err)
		}
		a, err := first.NeedsEnrichment(stored)
		if err != nil {
			t.Fatal(err)
		}
		b, err := second.NeedsEnrichment(stored)
		if err != nil {
			t.Fatal(err)
		}
		if !a && !b {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("queued document was not enriched by both providers")
		}
		time.Sleep(time.Millisecond)
	}
	if err := idx.EnrichDocuments(0); err == nil {
		t.Fatal("accepted invalid batch size")
	}
}
