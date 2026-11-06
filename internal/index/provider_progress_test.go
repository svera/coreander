package index

import (
	"testing"
	"time"

	"github.com/blevesearch/bleve/v2"
)

type progressCountingIndex struct {
	documentUpdateBaseIndex
	searches int
	counts   int
}

func (i *progressCountingIndex) Search(request *bleve.SearchRequest) (*bleve.SearchResult, error) {
	i.searches++
	return i.documentUpdateBaseIndex.Search(request)
}

func (i *progressCountingIndex) DocCount() (uint64, error) {
	i.counts++
	return i.documentUpdateBaseIndex.DocCount()
}

func TestProviderProgressCachesCounts(t *testing.T) {
	idx, documents := newTestTextRankIndexer(t, Config{
		DocumentEnrichers: []DocumentEnricher{&testDocumentEnricher{source: "dummy"}},
	})
	t.Cleanup(func() { _ = idx.Close() })
	counting := &progressCountingIndex{documentUpdateBaseIndex: documents}
	idx.documentsIdx = counting
	if err := IndexDocument(documents, "book", Document{ID: "book"}); err != nil {
		t.Fatal(err)
	}
	documentReads := 0
	check := func(wantTasks, wantQueries int) {
		t.Helper()
		tasks, err := idx.BackgroundProgress()
		if err != nil {
			t.Fatal(err)
		}
		if len(tasks) != wantTasks || counting.counts != wantQueries || counting.searches != wantQueries+documentReads {
			t.Fatalf("tasks=%+v counts=%d searches=%d; want %d tasks and %d queries",
				tasks, counting.counts, counting.searches, wantTasks, wantQueries)
		}
	}
	check(1, 1)
	check(1, 1)
	idx.providerProgressMu.Lock()
	idx.providerProgressUntil = time.Now().Add(-time.Second)
	idx.providerProgressMu.Unlock()
	check(1, 2)

	if err := idx.StoreDocumentData("book", "dummy", testProviderData{Enriched: true}); err != nil {
		t.Fatal(err)
	}
	// StoreDocumentData itself reads the document once.
	documentReads++
	check(0, 3)
	check(0, 3)
	if err := idx.UpdateDocument("book", func(update *DocumentUpdate) (bool, error) {
		update.DataSources = nil
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	documentReads++
	check(1, 4)
	check(1, 4)
}
