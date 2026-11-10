package index

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/svera/coreander/v5/internal/metadata"
)

func evidenceDocument(id string, phrases ...string) Document {
	return Document{
		ID: id, Slug: id,
		Metadata:        metadata.Metadata{Title: id, Format: "EPUB"},
		TextRankPhrases: phrases, TextRankEnriched: true,
	}
}

type similarityBaseIndex = bleve.Index

type similarityCountingIndex struct {
	similarityBaseIndex
	searches, internalReads int
}

func (i *similarityCountingIndex) Search(request *bleve.SearchRequest) (*bleve.SearchResult, error) {
	i.searches++
	return i.similarityBaseIndex.Search(request)
}

func (i *similarityCountingIndex) GetInternal(key []byte) ([]byte, error) {
	i.internalReads++
	return i.similarityBaseIndex.GetInternal(key)
}

func TestSimilarityEvidenceRejectsCommonAndSinglePhraseMatches(t *testing.T) {
	idx, documents := newTestTextRankIndexer(t, Config{
		CommonTextRankEntryRatio: 1,
		MaxSimilarityPhrases:     2,
	})
	t.Cleanup(func() { _ = idx.Close() })
	for i := range 200 {
		doc := evidenceDocument(fmt.Sprintf("filler-%d", i))
		if i < 21 {
			doc.TextRankPhrases = []string{"opened door", "closed eyes", "shook head"}
		}
		if err := IndexDocument(documents, doc.ID, doc); err != nil {
			t.Fatal(err)
		}
	}
	for _, doc := range []Document{
		evidenceDocument("reference", "opened door", "closed eyes", "atomic bomb", "los alamos", "atomic bomb"),
		evidenceDocument("strong", "atomic bomb", "los alamos"),
		evidenceDocument("single", "atomic bomb", "opened door", "closed eyes"),
		evidenceDocument("generic", "opened door", "closed eyes", "shook head"),
	} {
		if err := IndexDocument(documents, doc.ID, doc); err != nil {
			t.Fatal(err)
		}
	}
	// Requests before the first cleanup must not perform an on-demand scan.
	before, err := idx.Search(SearchFields{SimilarTo: "reference"}, 1, 10)
	if err != nil || len(before.Hits()) != 0 {
		t.Fatalf("before statistics: hits=%v error=%v", before.Hits(), err)
	}
	if err := idx.EnrichTextRankKeywords(25, 1); err != nil {
		t.Fatal(err)
	}
	counted := &similarityCountingIndex{similarityBaseIndex: documents}
	idx.documentsIdx = counted
	for _, size := range []int{1, 20} {
		results, err := idx.Search(SearchFields{SimilarTo: "reference"}, 1, size)
		if err != nil {
			t.Fatal(err)
		}
		if len(results.Hits()) != 1 || results.Hits()[0].Slug != "strong" {
			t.Fatalf("page size %d: expected only strong evidence, got %v", size, results.Hits())
		}
		if results.Candidates.Total() != 1 {
			t.Fatalf("weak candidates must be excluded before capping, total=%d", results.Candidates.Total())
		}
	}
	if counted.internalReads != 0 || counted.searches != 4 {
		t.Fatalf("cached requests should only read the reference and run the candidate query: internal reads=%d searches=%d", counted.internalReads, counted.searches)
	}
	// The recommendation filter must not strip keywords from ordinary search.
	generic, err := idx.Document("generic")
	if err != nil || !slices.Contains(generic.TextRankPhrases, "opened door") {
		t.Fatalf("recommendation filter changed stored keywords: %v, %v", generic.TextRankPhrases, err)
	}
	ordinary, err := idx.Search(SearchFields{Keywords: "opened door"}, 1, 100)
	if err != nil || len(ordinary.Hits()) == 0 {
		t.Fatalf("ordinary search lost generic keywords: hits=%d error=%v", len(ordinary.Hits()), err)
	}

	// A new indexer loads the persisted snapshot without keyword extraction.
	restarted := NewBleve(documents, idx.authorsIdx, nil, "", nil, Config{MaxSimilarityPhrases: 2})
	results, err := restarted.Search(SearchFields{SimilarTo: "reference"}, 1, 10)
	if err != nil || len(results.Hits()) != 1 || results.Hits()[0].Slug != "strong" {
		t.Fatalf("persisted statistics: hits=%v error=%v", results.Hits(), err)
	}
}

func TestSimilarityEvidenceNormalizesAndDeduplicatesPhrases(t *testing.T) {
	idx, documents := newTestTextRankIndexer(t, Config{CommonTextRankEntryRatio: 1})
	t.Cleanup(func() { _ = idx.Close() })
	for _, doc := range []Document{
		evidenceDocument("reference", "Átomic bomb", "atomic BOMB", "atomic-bomb", "los alamos"),
		evidenceDocument("duplicate", "atomic bomb", "ATOMIC BOMB"),
		evidenceDocument("strong", "atomic bomb", "LOS ALAMOS"),
	} {
		if err := IndexDocument(documents, doc.ID, doc); err != nil {
			t.Fatal(err)
		}
	}
	if err := idx.pruneCommonTextRankEntries(2); err != nil {
		t.Fatal(err)
	}
	stats, err := idx.similarityStatistics()
	if err != nil {
		t.Fatal(err)
	}
	if stats.PhraseCounts["atomic bomb"] != 3 || stats.PhraseCounts["los alamos"] != 2 {
		t.Fatalf("expected per-document normalized frequencies, got %v", stats.PhraseCounts)
	}
	results, err := idx.Search(SearchFields{SimilarTo: "reference"}, 1, 10)
	if err != nil || len(results.Hits()) != 1 || results.Hits()[0].Slug != "strong" {
		t.Fatalf("duplicates must not satisfy two-phrase minimum: hits=%v error=%v", results.Hits(), err)
	}
	// Even the best match must disappear if there is only one distinctive pair.
	results, err = idx.Search(SearchFields{SimilarTo: "duplicate"}, 1, 10)
	if err != nil || len(results.Hits()) != 0 {
		t.Fatalf("single-phrase reference: hits=%v error=%v", results.Hits(), err)
	}
}

func TestSimilarityPhraseFrequencyBoundaries(t *testing.T) {
	for _, tc := range []struct {
		documents uint64
		count     int
		eligible  bool
	}{
		{10, 2, true},
		{1000, 20, true},
		{1000, 21, false},
		{3000, 30, true},
		{3000, 31, false},
		{3000, 1, false},
	} {
		t.Run(fmt.Sprintf("%d-%d", tc.documents, tc.count), func(t *testing.T) {
			counts := eligibleSimilarityPhraseCounts(map[string]int{"atomic bomb": tc.count}, tc.documents)
			if _, got := counts["atomic bomb"]; got != tc.eligible {
				t.Fatalf("eligible=%v, want %v", got, tc.eligible)
			}
		})
	}
}

func TestSimilarityStatisticsRefreshAndErrors(t *testing.T) {
	idx, documents := newTestTextRankIndexer(t, Config{})
	t.Cleanup(func() { _ = idx.Close() })
	for _, doc := range []Document{evidenceDocument("a", "atomic bomb", "los alamos"), evidenceDocument("b", "atomic bomb", "los alamos")} {
		if err := IndexDocument(documents, doc.ID, doc); err != nil {
			t.Fatal(err)
		}
	}
	if err := idx.pruneCommonTextRankEntries(10); err != nil {
		t.Fatal(err)
	}
	if idx.commonTextRankEntriesNeedPrune() {
		t.Fatal("unchanged library must reuse statistics")
	}
	idx.commonTextRankEntryRatio = 0.1
	if !idx.commonTextRankEntriesNeedPrune() {
		t.Fatal("changed pruning ratio must refresh statistics")
	}
	if err := idx.pruneCommonTextRankEntries(10); err != nil {
		t.Fatal(err)
	}
	if idx.commonTextRankEntriesNeedPrune() {
		t.Fatal("cleanup must persist the new pruning ratio")
	}
	idx.minCommonTextRankAbsoluteCount++
	if !idx.commonTextRankEntriesNeedPrune() {
		t.Fatal("changed minimum must refresh statistics")
	}
	if err := documents.SetInternal(internalSimilarityPhraseStatistics, []byte("{")); err != nil {
		t.Fatal(err)
	}
	idx.similarityStatsLoaded = false
	if _, err := idx.Search(SearchFields{SimilarTo: "a"}, 1, 10); err == nil {
		t.Fatal("corrupt statistics must return an explicit error")
	}
	if !idx.commonTextRankEntriesNeedPrune() {
		t.Fatal("corrupt statistics must trigger background rebuilding")
	}
}

func BenchmarkSimilarityEvidenceQuery(b *testing.B) {
	documents, err := bleve.NewMemOnly(CreateDocumentsMapping())
	if err != nil {
		b.Fatal(err)
	}
	defer documents.Close()
	idx := &BleveIndexer{documentsIdx: documents, maxSimilarityPhrases: 60}
	stats := &similarityStatistics{DocumentCount: 37500, PhraseCounts: make(map[string]int), SubjectCounts: make(map[string]int)}
	doc := Document{}
	for i := range 500 {
		phrase := fmt.Sprintf("topic%d detail%d", i, i)
		doc.TextRankPhrases = append(doc.TextRankPhrases, phrase)
		stats.PhraseCounts[phrase] = 100
	}
	raw, err := json.Marshal(stats)
	if err != nil {
		b.Fatal(err)
	}
	if err := documents.SetInternal(internalSimilarityPhraseStatistics, raw); err != nil {
		b.Fatal(err)
	}
	if _, err := idx.similarityStatistics(); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		cached, err := idx.similarityStatistics()
		if err != nil {
			b.Fatal(err)
		}
		if _, err := idx.similarToQuery(doc, cached); err != nil {
			b.Fatal(err)
		}
	}
}
