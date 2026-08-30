package index

import (
	"strconv"
	"testing"

	"github.com/blevesearch/bleve/v2"
)

// TestCommonTextRankEntriesNeedPrune guards the gate EnrichTextRankKeywords
// uses to skip pruneCommonTextRankEntries's expensive whole-library,
// every-field rehydration when it wouldn't change anything: restarting the
// app against an already fully-enriched, unchanged library was making that
// unconditional scan run on every startup, which alone was enough to make
// the app unresponsive during startup (see EnrichTextRankKeywords).
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
