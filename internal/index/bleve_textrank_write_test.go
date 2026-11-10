package index

import (
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/blevesearch/bleve/v2"
)

// newTestTextRankIndexer builds an in-memory BleveIndexer for the pruning
// tests below, which otherwise all repeat the same documents/authors index
// setup with different Config values.
func newTestTextRankIndexer(t *testing.T, cfg Config) (idx *BleveIndexer, documentsIndexMem bleve.Index) {
	t.Helper()

	documentsIndexMem, err := bleve.NewMemOnly(CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}
	authorsIndexMem, err := bleve.NewMemOnly(CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}

	return NewBleve(documentsIndexMem, authorsIndexMem, nil, "", nil, cfg), documentsIndexMem
}

func TestTextRankMaintenanceSharesCountingPass(t *testing.T) {
	idx, documents := newTestTextRankIndexer(t, Config{CommonTextRankEntryRatio: 1})
	t.Cleanup(func() { _ = idx.Close() })
	for i := range 3 {
		doc := evidenceDocument(strconv.Itoa(i), "atomic bomb", "los alamos")
		doc.SubjectsSlugs = []string{"physics"}
		if err := IndexDocument(documents, doc.ID, doc); err != nil {
			t.Fatal(err)
		}
	}
	counted := &similarityCountingIndex{similarityBaseIndex: documents}
	idx.documentsIdx = counted
	if err := idx.maintainTextRankEntries(2); err != nil {
		t.Fatal(err)
	}
	if counted.searches != 2 {
		t.Fatalf("expected one shared counting pass (two batches), got %d searches", counted.searches)
	}
	stats, err := idx.loadSimilarityStatistics()
	if err != nil {
		t.Fatal(err)
	}
	if stats == nil || stats.PhraseCounts["atomic bomb"] != 3 || stats.SubjectCounts["physics"] != 3 {
		t.Fatalf("maintenance did not publish collected statistics: %v", stats)
	}
}

func TestTextRankPruningDoesNotPublishSimilarityStatistics(t *testing.T) {
	idx, documents := newTestTextRankIndexer(t, Config{
		CommonTextRankEntryRatio:       0.5,
		MinCommonTextRankAbsoluteCount: 1,
	})
	t.Cleanup(func() { _ = idx.Close() })
	for i := range 2 {
		doc := evidenceDocument(strconv.Itoa(i), "common phrase")
		if err := IndexDocument(documents, doc.ID, doc); err != nil {
			t.Fatal(err)
		}
	}
	search := func(from int) (*bleve.SearchResult, error) {
		request := bleve.NewSearchRequestOptions(bleve.NewMatchAllQuery(), 2, from, false)
		request.Fields = []string{"TextRankPhrases", "TextRankWords"}
		return documents.Search(request)
	}
	if err := idx.pruneCommonTextRankEntries(2, 2, map[string]int{"common phrase": 2}, nil, search); err != nil {
		t.Fatal(err)
	}
	doc, err := idx.documentByIndexID("0")
	if err != nil || len(doc.TextRankPhrases) != 0 {
		t.Fatalf("pruning did not remove common phrase: %v, %v", doc.TextRankPhrases, err)
	}
	for _, key := range [][]byte{internalSimilarityPhraseStatistics, internalDocCountAtLastPrune} {
		value, err := documents.GetInternal(key)
		if err != nil || len(value) != 0 {
			t.Fatalf("pruning must not publish metadata %q: %q, %v", key, value, err)
		}
	}
	if idx.similarityStatsLoaded || idx.similarityStats != nil {
		t.Fatal("pruning must not publish a similarity cache")
	}
}

// TestCommonTextRankEntriesNeedPrune guards the gate EnrichTextRankKeywords
// uses to skip maintainTextRankEntries when nothing changed.
func TestCommonTextRankEntriesNeedPrune(t *testing.T) {
	idx, documentsIndexMem := newTestTextRankIndexer(t, Config{
		PruneChangeTriggerRatio: 0.5,
	})

	for i := 0; i < 4; i++ {
		if err := IndexDocument(documentsIndexMem, strconv.Itoa(i), Document{ID: strconv.Itoa(i)}); err != nil {
			t.Fatal(err)
		}
	}

	if !idx.commonTextRankEntriesNeedPrune() {
		t.Error("expected pruning to be needed when no prune has ever run")
	}

	if err := idx.maintainTextRankEntries(10); err != nil {
		t.Fatal(err)
	}

	if idx.commonTextRankEntriesNeedPrune() {
		t.Error("expected pruning to be skipped when the library hasn't changed since the last prune")
	}

	// PruneChangeTriggerRatio is 0.5 of the 4 documents recorded above (i.e.
	// 2), so 3 more documents is enough to cross it.
	for i := 0; i < 3; i++ {
		if err := IndexDocument(documentsIndexMem, "extra"+strconv.Itoa(i), Document{ID: "extra" + strconv.Itoa(i)}); err != nil {
			t.Fatal(err)
		}
	}

	if !idx.commonTextRankEntriesNeedPrune() {
		t.Error("expected pruning to be needed again once the library changed by more than PruneChangeTriggerRatio")
	}
}

// TestPruningReportsProgress guards IndexingProgress surfacing Kind
// ProgressPruning while maintainTextRankEntries runs.
func TestPruningReportsProgress(t *testing.T) {
	idx, documentsIndexMem := newTestTextRankIndexer(t, Config{})

	const docCount = 200
	for i := 0; i < docCount; i++ {
		id := strconv.Itoa(i)
		document := Document{
			ID:              id,
			TextRankPhrases: []string{"phrase " + id},
			TextRankWords:   []string{"word" + id},
		}
		if err := IndexDocument(documentsIndexMem, id, document); err != nil {
			t.Fatal(err)
		}
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := idx.maintainTextRankEntries(10); err != nil {
			t.Error(err)
		}
	}()

	deadline := time.Now().Add(2 * time.Second)
	sawPruningProgress := false
	for time.Now().Before(deadline) {
		p, err := idx.IndexingProgress()
		if err != nil {
			t.Fatal(err)
		}
		if p.InProgress && p.Kind == ProgressPruning {
			sawPruningProgress = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	wg.Wait()

	if !sawPruningProgress {
		t.Fatal("expected IndexingProgress to report Kind=ProgressPruning while pruning was running")
	}

	if p, err := idx.IndexingProgress(); err != nil {
		t.Fatal(err)
	} else if p.InProgress {
		t.Error("expected pruning progress to no longer be in progress once maintainTextRankEntries returns")
	}
}

// TestPruneCommonTextRankEntriesRewritesCommonEntries guards the rewrite
// path: an entry shared by enough documents to cross the threshold should be
// stripped from all of them, while a unique entry survives.
func TestPruneCommonTextRankEntriesRewritesCommonEntries(t *testing.T) {
	idx, documentsIndexMem := newTestTextRankIndexer(t, Config{
		CommonTextRankEntryRatio:       0.5,
		MinCommonTextRankAbsoluteCount: 1,
	})

	const docCount = 10
	for i := 0; i < docCount; i++ {
		id := strconv.Itoa(i)
		document := Document{
			ID:              id,
			TextRankPhrases: []string{"common phrase", "commonword distinctive" + id, "unique phrase " + id},
			TextRankWords:   []string{"commonword", "unique" + id},
		}
		if err := IndexDocument(documentsIndexMem, id, document); err != nil {
			t.Fatal(err)
		}
	}

	if err := idx.maintainTextRankEntries(3); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < docCount; i++ {
		id := strconv.Itoa(i)
		document, err := idx.documentByIndexID(id)
		if err != nil {
			t.Fatalf("documentByIndexID(%q) returned an error: %s", id, err)
		}
		for _, phrase := range document.TextRankPhrases {
			if phrase == "common phrase" || phrase == "commonword distinctive"+id {
				t.Errorf("document %s: expected common phrase or phrase containing commonword to have been pruned, still present in %q", id, document.TextRankPhrases)
			}
		}
		for _, word := range document.TextRankWords {
			if word == "commonword" {
				t.Errorf("document %s: expected \"commonword\" to have been pruned, still present in %q", id, document.TextRankWords)
			}
		}
		wantPhrase := "unique phrase " + id
		if !slices.Contains(document.TextRankPhrases, wantPhrase) {
			t.Errorf("document %s: expected %q to survive pruning, got %q", id, wantPhrase, document.TextRankPhrases)
		}
	}
}
