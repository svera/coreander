package index_test

import (
	"slices"
	"testing"

	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/metadata"
)

// TestSameSubjectsMatchesStemmablePhrasesForLanguageRoutedDocuments guards
// against a real bug: documents with a Language set route to a per-language
// bleve document mapping (see Document.BleveType), and for languages with a
// stemming analyzer (Spanish, English, German, French, Italian, Portuguese -
// see noStopWordsFilters in bleve.go), TextRankKeywords used to be indexed
// with that stemming analyzer. But subjectsQuery's MatchPhraseQuery always
// analyzes its query terms with defaultAnalyzer, which never stems. A
// phrase like "potencias centrales" could get indexed in a stemmed form
// (e.g. "potencia central") while the query still looks for the literal
// unstemmed tokens, so the two silently never matched - even for documents
// whose own TextRankKeywords field contains that exact phrase. This
// evaded every other similarity test in this package because none of them
// set Language, so they always routed to DefaultMapping (already using
// defaultAnalyzer, unaffected). Setting Language: "es" here specifically
// exercises the per-language routing path.
func TestSameSubjectsMatchesStemmablePhrasesForLanguageRoutedDocuments(t *testing.T) {
	docA := index.Document{
		ID:               "a.epub",
		Slug:             "doc-a",
		Metadata:         metadata.Metadata{Title: "Doc A", Authors: []string{"Author One"}, Format: "EPUB", Language: "es"},
		AuthorsSlugs:     []string{"author-one"},
		TextRankKeywords: []string{"potencias centrales", "frente occidental"},
	}
	docSharesStemmablePhrase := index.Document{
		ID:               "shares.epub",
		Slug:             "shares-stemmable-phrase",
		Metadata:         metadata.Metadata{Title: "Shares Stemmable Phrase", Authors: []string{"Author Two"}, Format: "EPUB", Language: "es"},
		AuthorsSlugs:     []string{"author-two"},
		TextRankKeywords: []string{"potencias centrales", "frente occidental"},
	}
	docUnrelated := index.Document{
		ID:               "unrelated.epub",
		Slug:             "unrelated",
		Metadata:         metadata.Metadata{Title: "Unrelated", Authors: []string{"Author Three"}, Format: "EPUB", Language: "es"},
		AuthorsSlugs:     []string{"author-three"},
		TextRankKeywords: []string{"cooking recipes", "bread baking", "gardening tools"},
	}

	idx := newTextRankTestIndex(t, []index.Document{docA, docSharesStemmablePhrase, docUnrelated})

	got, err := idx.SameSubjects("doc-a", 10)
	if err != nil {
		t.Fatalf("SameSubjects returned an error: %s", err)
	}

	gotSlugs := slugsOf(got)
	if !slices.Contains(gotSlugs, "shares-stemmable-phrase") {
		t.Errorf("expected %q to match on its shared stemmable phrases, got %v", "shares-stemmable-phrase", gotSlugs)
	}
	if slices.Contains(gotSlugs, "unrelated") {
		t.Errorf("did not expect unrelated document to match, got %v", gotSlugs)
	}
}
