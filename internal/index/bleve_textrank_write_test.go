package index

import (
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/blevesearch/bleve/v2"
)

// TestCommonTextRankEntriesNeedPrune guards the gate EnrichTextRankKeywords
// uses to skip pruneCommonTextRankEntries when nothing changed.
func TestCommonTextRankEntriesNeedPrune(t *testing.T) {
	documentsIndexMem, err := bleve.NewMemOnly(CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}
	authorsIndexMem, err := bleve.NewMemOnly(CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}

	idx := NewBleve(documentsIndexMem, authorsIndexMem, nil, "", nil, Config{
		PruneChangeTriggerRatio: 0.5,
	})

	for i := 0; i < 4; i++ {
		if err := documentsIndexMem.Index(strconv.Itoa(i), Document{ID: strconv.Itoa(i)}); err != nil {
			t.Fatal(err)
		}
	}

	if !idx.commonTextRankEntriesNeedPrune() {
		t.Error("expected pruning to be needed when no prune has ever run")
	}

	docCount, err := documentsIndexMem.DocCount()
	if err != nil {
		t.Fatal(err)
	}
	if err := documentsIndexMem.SetInternal(internalDocCountAtLastPrune, []byte(strconv.FormatUint(docCount, 10))); err != nil {
		t.Fatal(err)
	}

	if idx.commonTextRankEntriesNeedPrune() {
		t.Error("expected pruning to be skipped when the library hasn't changed since the last prune")
	}

	// PruneChangeTriggerRatio is 0.5 of the 4 documents recorded above (i.e.
	// 2), so 3 more documents is enough to cross it.
	for i := 0; i < 3; i++ {
		if err := documentsIndexMem.Index("extra"+strconv.Itoa(i), Document{ID: "extra" + strconv.Itoa(i)}); err != nil {
			t.Fatal(err)
		}
	}

	if !idx.commonTextRankEntriesNeedPrune() {
		t.Error("expected pruning to be needed again once the library changed by more than PruneChangeTriggerRatio")
	}
}

// TestPruningReportsProgress guards IndexingProgress surfacing Kind
// ProgressPruning while pruneCommonTextRankEntries runs.
func TestPruningReportsProgress(t *testing.T) {
	documentsIndexMem, err := bleve.NewMemOnly(CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}
	authorsIndexMem, err := bleve.NewMemOnly(CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}

	idx := NewBleve(documentsIndexMem, authorsIndexMem, nil, "", nil, Config{})

	const docCount = 200
	for i := 0; i < docCount; i++ {
		id := strconv.Itoa(i)
		document := Document{
			ID:              id,
			TextRankPhrases: []string{"phrase " + id},
			TextRankWords:   []string{"word" + id},
		}
		if err := documentsIndexMem.Index(id, document); err != nil {
			t.Fatal(err)
		}
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := idx.pruneCommonTextRankEntries(10); err != nil {
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
		t.Error("expected pruning progress to no longer be in progress once pruneCommonTextRankEntries returns")
	}
}

// TestPruneCommonTextRankEntriesRewritesCommonEntries guards the rewrite
// path: an entry shared by enough documents to cross the threshold should be
// stripped from all of them, while a unique entry survives.
func TestPruneCommonTextRankEntriesRewritesCommonEntries(t *testing.T) {
	documentsIndexMem, err := bleve.NewMemOnly(CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}
	authorsIndexMem, err := bleve.NewMemOnly(CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}

	idx := NewBleve(documentsIndexMem, authorsIndexMem, nil, "", nil, Config{
		CommonTextRankEntryRatio:       0.5,
		MinCommonTextRankAbsoluteCount: 1,
	})

	const docCount = 10
	for i := 0; i < docCount; i++ {
		id := strconv.Itoa(i)
		document := Document{
			ID:              id,
			TextRankPhrases: []string{"common phrase", "unique phrase " + id},
			TextRankWords:   []string{"commonword", "unique" + id},
		}
		if err := documentsIndexMem.Index(id, document); err != nil {
			t.Fatal(err)
		}
	}

	if err := idx.pruneCommonTextRankEntries(3); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < docCount; i++ {
		id := strconv.Itoa(i)
		document, err := idx.documentByIndexID(id)
		if err != nil {
			t.Fatalf("documentByIndexID(%q) returned an error: %s", id, err)
		}
		for _, phrase := range document.TextRankPhrases {
			if phrase == "common phrase" {
				t.Errorf("document %s: expected \"common phrase\" to have been pruned, still present in %q", id, document.TextRankPhrases)
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
