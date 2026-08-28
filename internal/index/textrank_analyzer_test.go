package index_test

import (
	"slices"
	"testing"

	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/metadata"
)

// TestSearchSimilarToMatchesStemmablePhrasesForLanguageRoutedDocuments guards
// against a real bug: documents with a Language set route to a per-language
// bleve document mapping (see Document.BleveType), and for languages with a
// stemming analyzer (Spanish, English, German, French, Italian, Portuguese -
// see noStopWordsFilters in bleve.go), TextRankPhrases used to be indexed
// with that stemming analyzer. But similarToQuery's MatchPhraseQuery always
// analyzes its query terms with defaultAnalyzer, which never stems. A
// phrase like "potencias centrales" could get indexed in a stemmed form
// (e.g. "potencia central") while the query still looks for the literal
// unstemmed tokens, so the two silently never matched - even for documents
// whose own TextRankPhrases field contains that exact phrase. This
// evaded every other similarity test in this package because none of them
// set Language, so they always routed to DefaultMapping (already using
// defaultAnalyzer, unaffected). Setting Language: "es" here specifically
// exercises the per-language routing path.
func TestSearchSimilarToMatchesStemmablePhrasesForLanguageRoutedDocuments(t *testing.T) {
	docA := index.Document{
		ID:              "a.epub",
		Slug:            "doc-a",
		Metadata:        metadata.Metadata{Title: "Doc A", Authors: []string{"Author One"}, Format: "EPUB", Language: "es"},
		AuthorsSlugs:    []string{"author-one"},
		TextRankPhrases: []string{"potencias centrales", "frente occidental"},
	}
	docSharesStemmablePhrase := index.Document{
		ID:              "shares.epub",
		Slug:            "shares-stemmable-phrase",
		Metadata:        metadata.Metadata{Title: "Shares Stemmable Phrase", Authors: []string{"Author Two"}, Format: "EPUB", Language: "es"},
		AuthorsSlugs:    []string{"author-two"},
		TextRankPhrases: []string{"potencias centrales", "frente occidental"},
	}
	docUnrelated := index.Document{
		ID:              "unrelated.epub",
		Slug:            "unrelated",
		Metadata:        metadata.Metadata{Title: "Unrelated", Authors: []string{"Author Three"}, Format: "EPUB", Language: "es"},
		AuthorsSlugs:    []string{"author-three"},
		TextRankPhrases: []string{"cooking recipes", "bread baking", "gardening tools"},
	}

	idx := newTextRankTestIndex(t, []index.Document{docA, docSharesStemmablePhrase, docUnrelated})

	res, err := idx.Search(index.SearchFields{SimilarTo: "doc-a"}, 1, 10)
	if err != nil {
		t.Fatalf("Search returned an error: %s", err)
	}

	gotSlugs := slugsOf(res.Hits())
	if !slices.Contains(gotSlugs, "shares-stemmable-phrase") {
		t.Errorf("expected %q to match on its shared stemmable phrases, got %v", "shares-stemmable-phrase", gotSlugs)
	}
	if slices.Contains(gotSlugs, "unrelated") {
		t.Errorf("did not expect unrelated document to match, got %v", gotSlugs)
	}
}

// TestSearchMatchesStemmedTextRankWords guards against the opposite mismatch
// from the one above: TextRankWords is mapped to the document's per-language
// analyzer (see CreateDocumentsMapping), specifically so a general keyword
// search's query terms - also analyzed with that same per-language analyzer
// in composeQuery - can match a stored word in a different
// singular/plural or other inflected form.
func TestSearchMatchesStemmedTextRankWords(t *testing.T) {
	docA := index.Document{
		ID:            "a.epub",
		Slug:          "doc-a",
		Metadata:      metadata.Metadata{Title: "Doc A", Authors: []string{"Author One"}, Format: "EPUB", Language: "es"},
		AuthorsSlugs:  []string{"author-one"},
		TextRankWords: []string{"potencias"},
	}
	docUnrelated := index.Document{
		ID:            "unrelated.epub",
		Slug:          "unrelated",
		Metadata:      metadata.Metadata{Title: "Unrelated", Authors: []string{"Author Two"}, Format: "EPUB", Language: "es"},
		AuthorsSlugs:  []string{"author-two"},
		TextRankWords: []string{"jardin"},
	}

	idx := newTextRankTestIndex(t, []index.Document{docA, docUnrelated})

	got, err := idx.Search(index.SearchFields{Keywords: "potencia"}, 1, 10)
	if err != nil {
		t.Fatalf("Search returned an error: %s", err)
	}

	gotSlugs := slugsOf(got.Hits())
	if !slices.Contains(gotSlugs, "doc-a") {
		t.Errorf("expected singular query %q to match a document whose TextRankWords stores the plural %q, got %v", "potencia", "potencias", gotSlugs)
	}
	if slices.Contains(gotSlugs, "unrelated") {
		t.Errorf("did not expect unrelated document to match, got %v", gotSlugs)
	}
}
