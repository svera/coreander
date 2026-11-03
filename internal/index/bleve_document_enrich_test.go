package index

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/svera/coreander/v5/internal/metadata"
)

type testDocumentEnricher struct {
	source  string
	calls   atomic.Int32
	failure error
}

type blockingDocumentEnricher struct {
	started chan struct{}
	release chan struct{}
}

func (*blockingDocumentEnricher) Source() string { return "blocking" }

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
	done := make(chan error, 1)
	go func() { done <- idx.enrichProviderBatch(provider, documents) }()
	defer func() {
		close(provider.release)
		if err := <-done; err != nil {
			t.Errorf("enrichment failed: %v", err)
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

func (p *testDocumentEnricher) Source() string { return p.source }

func (p *testDocumentEnricher) NeedsEnrichment(document Document) (bool, error) {
	data, err := DataSourceData[struct{ Enriched bool }](document, p.source)
	return !data.Enriched, err
}

func (p *testDocumentEnricher) Enrich(_ context.Context, idx *BleveIndexer, document Document) error {
	p.calls.Add(1)
	if p.failure != nil {
		return p.failure
	}
	return idx.StoreDocumentData(document.ID, p.source, map[string]any{
		"enriched": true, "reference": p.source + "-reference",
	})
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
