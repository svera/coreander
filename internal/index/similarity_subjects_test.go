package index

import (
	"fmt"
	"maps"
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
			if err := idx.maintainTextRankEntries(100); err != nil {
				t.Fatal(err)
			}
			stats, err := idx.similarityStatistics()
			if err != nil {
				t.Fatal(err)
			}
			if stats.SubjectCounts["novela"] < 40 || stats.SubjectCounts["novela"] > 45 {
				t.Fatalf("subjects must count once per document, got %d", stats.SubjectCounts["novela"])
			}
			want := []string{"shared", "mixed-shared", "generic", "missing", "empty"}
			if len(uncommonSimilaritySubjects(referenceSubjects, stats.uncommonSubjects)) == 0 {
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
			restarted := NewBleve(documents, idx.authorsIdx, nil, "", nil, Config{CommonTextRankEntryRatio: 1})
			if restarted.commonTextRankEntriesNeedPrune() {
				t.Fatal("restart should reuse statistics for the unchanged library")
			}
			restored, err := restarted.similarityStatistics()
			if err != nil {
				t.Fatal(err)
			}
			if restored == nil || !maps.Equal(restored.SubjectCounts, stats.SubjectCounts) ||
				!slices.Equal(restored.uncommonSubjects, stats.uncommonSubjects) {
				t.Fatalf("restart must restore subject counts and classification, got %v", restored)
			}
			results, err := restarted.Search(SearchFields{SimilarTo: source.Slug, SortBy: DefaultDocumentSortBy}, 1, 100)
			if err != nil {
				t.Fatal(err)
			}
			var reloaded []string
			for _, hit := range results.Hits() {
				reloaded = append(reloaded, hit.Slug)
			}
			if results.Candidates.Total() != len(want) || !slices.Equal(reloaded, full) {
				t.Fatalf("restart changed recommendations: got %v, want %v", reloaded, full)
			}
			slices.Sort(full)
			slices.Sort(want)
			if !slices.Equal(full, want) {
				t.Fatalf("got %v, want %v", full, want)
			}
		})
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
				got := uncommonSimilaritySubjects([]string{label, label, ""}, stats.uncommonSubjects)
				if tc.common && len(got) != 0 || !tc.common && !slices.Equal(got, []string{label}) {
					t.Fatalf("%s common=%v, got %v", label, tc.common, got)
				}
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
	if err := idx.maintainTextRankEntries(100); err != nil {
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
	if err := idx.maintainTextRankEntries(100); err != nil {
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
	if err := idx.maintainTextRankEntries(100); err != nil {
		t.Fatal(err)
	}
	results, err = idx.Search(SearchFields{SimilarTo: "reference"}, 1, 10)
	if err != nil || len(results.Hits()) != 2 {
		t.Fatalf("counted different subject should be filtered: hits=%v error=%v", results.Hits(), err)
	}
}
