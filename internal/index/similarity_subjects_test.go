package index

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/blevesearch/bleve/v2"
)

func TestSimilaritySubjectFilter(t *testing.T) {
	for _, referenceSubjects := range [][]string{
		{"novela", "ciencia-ficcion"},
		{"novela"},
		nil,
	} {
		t.Run(fmt.Sprint(referenceSubjects), func(t *testing.T) {
			idx, documents := newTestTextRankIndexer(t, Config{CommonTextRankEntryRatio: 1})
			t.Cleanup(func() { _ = idx.Close() })
			for i := range 100 {
				filler := evidenceDocument(fmt.Sprintf("filler-%d", i))
				if i < 40 {
					filler.SubjectsSlugs = []string{"novela", "fiction", "literature", "novela"}
				}
				if err := IndexDocument(documents, filler.ID, filler); err != nil {
					t.Fatal(err)
				}
			}
			source := evidenceDocument("reference", "atomic bomb", "los alamos")
			source.SubjectsSlugs = referenceSubjects
			if err := IndexDocument(documents, source.ID, source); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				id       string
				subjects []string
			}{
				{"shared", []string{"ciencia-ficcion"}},
				{"mixed-shared", []string{"novela", "ciencia-ficcion", "fantastico"}},
				{"different", []string{"historia"}},
				{"mixed-different", []string{"novela", "historia"}},
				{"generic", []string{"novela", "fiction", "literature"}},
				{"missing", nil},
				{"empty", []string{""}},
				{"romance", []string{"romance"}},
			} {
				doc := evidenceDocument(tc.id, source.TextRankPhrases...)
				doc.SubjectsSlugs = tc.subjects
				if err := IndexDocument(documents, doc.ID, doc); err != nil {
					t.Fatal(err)
				}
			}
			weak := evidenceDocument("subject-only", "atomic bomb")
			weak.SubjectsSlugs = []string{"ciencia-ficcion"}
			if err := IndexDocument(documents, weak.ID, weak); err != nil {
				t.Fatal(err)
			}
			if err := idx.pruneCommonTextRankEntries(100); err != nil {
				t.Fatal(err)
			}
			stats, err := idx.similarityStatistics()
			if err != nil {
				t.Fatal(err)
			}
			if stats.SubjectCounts["novela"] < 40 || stats.SubjectCounts["novela"] > 45 {
				t.Fatalf("subjects must count once per document, got %d", stats.SubjectCounts["novela"])
			}
			counted := &similarityCountingIndex{similarityBaseIndex: documents}
			idx.documentsIdx = counted
			want := []string{"shared", "mixed-shared", "generic", "missing", "empty"}
			if len(usefulSimilaritySubjects(referenceSubjects, stats.usefulSubjects)) == 0 {
				want = append(want, "different", "mixed-different", "romance")
			}
			var full []string
			for page := 1; page <= 3; page++ {
				results, err := idx.Search(SearchFields{SimilarTo: source.Slug, SortBy: DefaultDocumentSortBy}, page, 3)
				if err != nil {
					t.Fatal(err)
				}
				if results.Candidates.Total() != len(want) {
					t.Fatalf("candidate total=%d, want %d", results.Candidates.Total(), len(want))
				}
				for _, hit := range results.Hits() {
					full = append(full, hit.Slug)
				}
			}
			preview, err := idx.Search(SearchFields{SimilarTo: source.Slug, SortBy: DefaultDocumentSortBy}, 1, 4)
			if err != nil {
				t.Fatal(err)
			}
			for i, hit := range preview.Hits() {
				if hit.Slug != full[i] {
					t.Fatal("preview and paginated search disagree")
				}
			}
			slices.Sort(full)
			slices.Sort(want)
			if !slices.Equal(full, want) {
				t.Fatalf("got %v, want %v", full, want)
			}
			if counted.internalReads != 0 || counted.searches != 8 {
				t.Fatalf("cached subject filtering added lookups: internal reads=%d searches=%d", counted.internalReads, counted.searches)
			}
		})
	}
}

func TestUsefulSimilaritySubjectsQuery(t *testing.T) {
	idx, documents := newTestTextRankIndexer(t, Config{})
	t.Cleanup(func() { _ = idx.Close() })
	for i, subject := range []string{"common-a", "common-z", "", "aaa", "zzz", "fantastico", "romance", "historical-fiction"} {
		doc := evidenceDocument(fmt.Sprint(i))
		doc.SubjectsSlugs = []string{subject}
		if err := IndexDocument(documents, doc.ID, doc); err != nil {
			t.Fatal(err)
		}
	}
	result, err := documents.Search(bleve.NewSearchRequestOptions(usefulSimilaritySubjectsQuery([]string{"aaa", "zzz", "fantastico", "romance", "historical-fiction"}), 100, 0, false))
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 5 {
		t.Fatalf("expected only five useful subjects, got %d", result.Total)
	}
	result, err = documents.Search(bleve.NewSearchRequestOptions(usefulSimilaritySubjectsQuery(nil), 100, 0, false))
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 0 {
		t.Fatalf("no known useful subjects: total=%d", result.Total)
	}
}

func TestSimilaritySubjectFrequencyBoundaries(t *testing.T) {
	for _, tc := range []struct {
		documents uint64
		count     int
		common    bool
	}{
		{10, 10, false},
		{100, 20, false},
		{100, 21, true},
		{1000, 200, false},
		{1000, 201, true},
	} {
		t.Run(fmt.Sprintf("%d-%d", tc.documents, tc.count), func(t *testing.T) {
			// Classification depends only on frequency, not label or language.
			stats := &similarityStatistics{
				DocumentCount: tc.documents,
				SubjectCounts: map[string]int{"novela": tc.count, "science-fiction": tc.count, "arbitrary-label": tc.count, "": tc.count},
			}
			stats.prepareSubjectFrequencies()
			for _, label := range []string{"novela", "science-fiction", "arbitrary-label"} {
				got := usefulSimilaritySubjects([]string{label, label, ""}, stats.usefulSubjects)
				if tc.common && len(got) != 0 || !tc.common && !slices.Equal(got, []string{label}) {
					t.Fatalf("%s common=%v, got %v", label, tc.common, got)
				}
			}
			if got := usefulSimilaritySubjects([]string{"previously-unseen"}, stats.usefulSubjects); len(got) != 0 {
				t.Fatal("unseen subjects should use phrase-only fallback until counted")
			}
		})
	}
}

func TestSimilaritySubjectsDoNotBoostScores(t *testing.T) {
	idx, documents := newTestTextRankIndexer(t, Config{CommonTextRankEntryRatio: 1})
	t.Cleanup(func() { _ = idx.Close() })
	reference := evidenceDocument("reference", "atomic bomb", "los alamos", "nuclear physics")
	for _, doc := range []Document{
		reference,
		evidenceDocument("shared", reference.TextRankPhrases...),
		evidenceDocument("missing", reference.TextRankPhrases...),
		evidenceDocument("different", reference.TextRankPhrases...),
	} {
		switch doc.ID {
		case "shared":
			doc.SubjectsSlugs = []string{"ciencia-ficcion"}
		case "different":
			doc.SubjectsSlugs = []string{"historia"}
		}

		if err := IndexDocument(documents, doc.ID, doc); err != nil {
			t.Fatal(err)
		}
	}
	if err := idx.pruneCommonTextRankEntries(100); err != nil {
		t.Fatal(err)
	}
	stats, err := idx.similarityStatistics()
	if err != nil {
		t.Fatal(err)
	}
	scores := func(subjects []string) map[string]float64 {
		t.Helper()
		reference.SubjectsSlugs = subjects
		q, err := idx.similarToQuery(reference, stats)
		if err != nil {
			t.Fatal(err)
		}
		matches, err := documents.Search(bleve.NewSearchRequestOptions(q, 10, 0, false))
		if err != nil {
			t.Fatal(err)
		}
		found := make(map[string]float64)
		for _, hit := range matches.Hits {
			found[hit.ID] = hit.Score
		}
		return found
	}
	before, after := scores(nil), scores([]string{"ciencia-ficcion"})
	if len(before) != 3 || len(after) != 2 {
		t.Fatalf("unexpected matches: before=%v after=%v", before, after)
	}
	for id, score := range after {
		if score != before[id] {
			t.Fatalf("subject filter changed %s score from %f to %f", id, before[id], score)
		}
	}
}

func TestSimilaritySubjectStatisticsUpgradeAndRestart(t *testing.T) {
	idx, documents := newTestTextRankIndexer(t, Config{CommonTextRankEntryRatio: 1})
	t.Cleanup(func() { _ = idx.Close() })
	for i := range 30 {
		doc := evidenceDocument(fmt.Sprint(i), "atomic bomb", "los alamos")
		doc.SubjectsSlugs = []string{"arbitrary-common-label", "arbitrary-common-label"}
		if err := IndexDocument(documents, doc.ID, doc); err != nil {
			t.Fatal(err)
		}
	}
	// Simulate an existing phrase-only snapshot from before subject counting.
	oldStats := &similarityStatistics{
		DocumentCount: 30,
		PhraseCounts:  map[string]int{},
		PruneRatio:    1,
		PruneMinimum:  DefaultMinCommonTextRankAbsoluteCount,
	}
	raw, err := json.Marshal(oldStats)
	if err != nil {
		t.Fatal(err)
	}
	if err := documents.SetInternal(internalSimilarityPhraseStatistics, raw); err != nil {
		t.Fatal(err)
	}
	if err := documents.SetInternal(internalDocCountAtLastPrune, []byte("30")); err != nil {
		t.Fatal(err)
	}
	if !idx.commonTextRankEntriesNeedPrune() {
		t.Fatal("missing subject counts must trigger cleanup on an unchanged library")
	}
	stats, err := idx.similarityStatistics()
	if err != nil || stats != nil {
		t.Fatalf("recommendations must wait for subject statistics: stats=%v error=%v", stats, err)
	}
	if err := idx.EnrichTextRankKeywords(10, 1); err != nil {
		t.Fatal(err)
	}
	if idx.commonTextRankEntriesNeedPrune() {
		t.Fatal("cleanup should persist subject statistics even without new documents")
	}
	if idx.similarityStats.SubjectCounts["arbitrary-common-label"] != 30 {
		t.Fatalf("incorrect subject count: %v", idx.similarityStats.SubjectCounts)
	}
	// Clear the process cache to exercise the same disk-loading path as restart.
	idx.similarityStats, idx.similarityStatsLoaded = nil, false
	stats, err = idx.similarityStatistics()
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.usefulSubjects) != 0 || stats.SubjectCounts["arbitrary-common-label"] != 30 {
		t.Fatalf("restart must restore frequency classification, got %v", stats)
	}
}

func TestSimilarityUnseenSubjectsUsePhraseFallback(t *testing.T) {
	idx, documents := newTestTextRankIndexer(t, Config{CommonTextRankEntryRatio: 1})
	t.Cleanup(func() { _ = idx.Close() })
	for _, id := range []string{"reference", "shared", "different", "missing"} {
		doc := evidenceDocument(id, "atomic bomb", "los alamos")
		if id == "reference" || id == "shared" {
			doc.SubjectsSlugs = []string{"known-topic"}
		} else if id == "different" {
			doc.SubjectsSlugs = []string{"different-topic"}
		}
		if err := IndexDocument(documents, doc.ID, doc); err != nil {
			t.Fatal(err)
		}
	}
	if err := idx.pruneCommonTextRankEntries(100); err != nil {
		t.Fatal(err)
	}
	changed := evidenceDocument("different", "atomic bomb", "los alamos")
	changed.SubjectsSlugs = []string{"new-topic"}
	if err := IndexDocument(documents, changed.ID, changed); err != nil {
		t.Fatal(err)
	}
	results, err := idx.Search(SearchFields{SimilarTo: "reference"}, 1, 10)
	if err != nil || len(results.Hits()) != 3 {
		t.Fatalf("candidate with unseen subject should fall back to phrases: hits=%v error=%v", results.Hits(), err)
	}
	results, err = idx.Search(SearchFields{SimilarTo: "different"}, 1, 10)
	if err != nil || len(results.Hits()) != 3 {
		t.Fatalf("reference with unseen subject should fall back to phrases: hits=%v error=%v", results.Hits(), err)
	}
	if err := idx.pruneCommonTextRankEntries(100); err != nil {
		t.Fatal(err)
	}
	results, err = idx.Search(SearchFields{SimilarTo: "reference"}, 1, 10)
	if err != nil || len(results.Hits()) != 2 {
		t.Fatalf("counted different subject should be filtered: hits=%v error=%v", results.Hits(), err)
	}
}
