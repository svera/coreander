package index

import (
	"fmt"
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/svera/coreander/v5/internal/metadata"
)

// TestAddFiltersDoesNotAffectScoring guards against addFilters' filter
// clauses (language here, but the same fix applies to subjects/dates/
// reading time/pages/illustrated-only) injecting their own relevance score
// into results. filtersQuery is a ConjunctionQuery, which sums the score of
// every matching clause, so an unboosted filter would add its own score
// (e.g. a PrefixQuery's IDF, which varies per document depending on how many
// other documents share its exact field value - unrelated to relevance) on
// top of whatever relevance query filtersQuery also holds. A document could
// then rank above a more relevant one purely because its language tag
// happens to be rarer, which is what a user reported seeing after applying
// the language filter on a "similar to" search. Filters must only narrow
// which documents match, never change their score.
func TestAddFiltersDoesNotAffectScoring(t *testing.T) {
	docA := Document{
		ID:              "a.epub",
		Slug:            "doc-a",
		Metadata:        metadata.Metadata{Title: "Doc A", Format: "EPUB", Language: "es"},
		AuthorsSlugs:    []string{"author-one"},
		TextRankPhrases: []string{"oppenheimer manhattan", "project atomic", "los alamos", "physics nuclear"},
	}
	docStrongMatch := Document{
		ID:              "strong.epub",
		Slug:            "strong-match",
		Metadata:        metadata.Metadata{Title: "Strong Match", Format: "EPUB", Language: "es-ES"},
		AuthorsSlugs:    []string{"author-two"},
		TextRankPhrases: []string{"oppenheimer manhattan", "project atomic", "los alamos"},
	}
	docWeakMatch := Document{
		ID:           "weak.epub",
		Slug:         "weak-match",
		Metadata:     metadata.Metadata{Title: "Weak Match", Format: "EPUB", Language: "es-MX"},
		AuthorsSlugs: []string{"author-three"},
		// Shares only one pair with doc-a: a genuinely weaker match than
		// strong-match. Its language tag "es-MX" is unique in this small
		// library (unlike strong-match's "es-ES", shared with 20 filler
		// documents below), so an unboosted language filter would give it a
		// disproportionate IDF-based score boost unrelated to its relevance.
		TextRankPhrases: []string{"physics nuclear", "cooking recipes", "bread baking", "gardening tools"},
	}

	docs := []Document{docA, docStrongMatch, docWeakMatch}
	for i := range 20 {
		docs = append(docs, Document{
			ID:           fmt.Sprintf("filler%d.epub", i),
			Slug:         fmt.Sprintf("filler-%d", i),
			Metadata:     metadata.Metadata{Title: fmt.Sprintf("Filler %d", i), Format: "EPUB", Language: "es-ES"},
			AuthorsSlugs: []string{"filler-author"},
		})
	}

	documentsIndexMem, err := bleve.NewMemOnly(CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}
	authorsIndexMem, err := bleve.NewMemOnly(CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}
	idx := NewBleve(documentsIndexMem, authorsIndexMem, nil, "", nil, Config{})
	for _, doc := range docs {
		if err := documentsIndexMem.Index(doc.ID, doc); err != nil {
			t.Fatal(err)
		}
	}

	scoresFor := func(searchFields SearchFields) map[string]float64 {
		filtersQuery := bleve.NewConjunctionQuery()
		filtersQuery.AddQuery(idx.similarToQuery(docA))
		idx.addFilters(searchFields, filtersQuery)

		req := bleve.NewSearchRequestOptions(filtersQuery, 50, 0, false)
		req.Fields = []string{"Slug"}
		res, err := idx.documentsIdx.Search(req)
		if err != nil {
			t.Fatal(err)
		}
		scores := make(map[string]float64, len(res.Hits))
		for _, h := range res.Hits {
			scores[h.Fields["Slug"].(string)] = h.Score
		}
		return scores
	}

	without := scoresFor(SearchFields{})
	with := scoresFor(SearchFields{Language: "es"})

	for slug, wantScore := range without {
		gotScore, ok := with[slug]
		if !ok {
			t.Errorf("%q matched without the language filter but not with it", slug)
			continue
		}
		if gotScore != wantScore {
			t.Errorf("expected the language filter to leave %q's score unchanged, got %f before and %f after", slug, wantScore, gotScore)
		}
	}
}
