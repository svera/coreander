package index

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
	"github.com/svera/coreander/v5/internal/metadata"
)

type blockingGoodReadsSource struct {
	started chan struct{}
	release chan struct{}
}

func (s *blockingGoodReadsSource) SearchBooks(ctx context.Context, _ string) (biblioreads.SearchBooksResponse, error) {
	s.started <- struct{}{}
	select {
	case <-s.release:
		return biblioreads.SearchBooksResponse{Result: goodReadsTestResults}, nil
	case <-ctx.Done():
		return biblioreads.SearchBooksResponse{}, ctx.Err()
	}
}

func TestGoodReadsEnrichmentUsesBoundedConcurrency(t *testing.T) {
	source := &blockingGoodReadsSource{
		started: make(chan struct{}, 8),
		release: make(chan struct{}),
	}
	idx := newGoodReadsTestIndexer(t, source)
	var documents []Document
	for i := range 8 {
		doc := Document{
			ID:       fmt.Sprintf("book-%d.pdf", i),
			Metadata: metadata.Metadata{Title: "La Reina Dragon", Authors: []string{"Alice Borchardt"}},
		}
		if err := idx.documentsIdx.Index(doc.ID, doc); err != nil {
			t.Fatal(err)
		}
		documents = append(documents, doc)
	}
	done := make(chan error, 1)
	go func() { done <- idx.enrichGoodReadsBatch(documents) }()
	defer func() {
		close(source.release)
		if err := <-done; err != nil {
			t.Errorf("enrichment failed: %v", err)
		}
	}()
	for range goodReadsEnrichWorkers {
		select {
		case <-source.started:
		case <-time.After(time.Second):
			t.Fatal("expected concurrent Goodreads lookups")
		}
	}
	select {
	case <-source.started:
		t.Fatal("enrichment exceeded its concurrency bound")
	case <-time.After(30 * time.Millisecond):
	}
}
