package index_test

import (
	"fmt"
	"html/template"
	"testing"
	"time"

	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/metadata"
)

// These are small, hand-selected subsets of the phrases displayed by the local
// library's TextRank debug pages, not complete book indexes. Descriptions are
// original summaries. Judgments reflect thematic similarity, not author or
// series membership. This fixture is a diagnostic, not a representative corpus.
func similarityRelevanceDocuments() []index.Document {
	type book struct {
		slug, title, author, authorSlug, description string
		phrases                                      []string
	}
	books := []book{
		{
			"richard-matheson-el-increible-hombre-menguante", "El increíble hombre menguante", "Richard Matheson", "richard-matheson",
			"Un hombre que se encoge lucha por sobrevivir en un mundo peligroso y gigantesco.",
			[]string{"media vuelta", "permaneció inmóvil", "echó correr", "abrió puerta", "cerró ojos", "caja cartón", "puso pie", "sacudió cabeza", "hombre menguante", "respiró hondo", "voz baja", "mano derecha", "acto seguido"},
		},
		{
			"isaac-asimov-viaje-alucinante", "Viaje alucinante", "Isaac Asimov", "isaac-asimov",
			"Una tripulación reducida a tamaño microscópico lucha por sobrevivir dentro del cuerpo humano.",
			[]string{"doctor duval", "doctor michaels", "torrente sanguíneo", "sistema circulatorio", "glóbulo blanco", "glóbulos rojos", "oído interno"},
		},
		{
			"jonathan-swift-los-viajes-de-gulliver", "Los viajes de Gulliver", "Jonathan Swift", "jonathan-swift",
			"Un viajero descubre un mundo de personas gigantes y diminutas.",
			[]string{"majestad imperial", "viajes gulliver", "rocín alazán", "cara manos", "hombre montaña", "media hora", "pies cuadrados", "mano derecha"},
		},
		{
			"h-g-wells-el-hombre-invisible", "El hombre invisible", "H. G. Wells", "h-g-wells",
			"Un científico invisible lucha por sobrevivir tras una transformación.",
			[]string{"señora hall", "doctor kemp", "thomas marvel", "acto seguido", "teddy henfrey", "coach horses", "señora bunting"},
		},
		{
			"j-r-ward-amante-desatado", "Amante Desatado", "J. R. Ward", "j-r-ward",
			"Una historia romántica paranormal sobre vampiros.",
			[]string{"virgen escribana", "frunció ceño", "red sox", "media vuelta", "puso pie", "cerró ojos", "voz baja", "sacudió cabeza", "abrió puerta"},
		},
		{
			"j-r-ward-amante-liberada", "Amante Liberada", "J. R. Ward", "j-r-ward",
			"Una historia romántica paranormal sobre vampiros.",
			[]string{"media vuelta", "cerró ojos", "miró reojo", "sacudió cabeza", "frunció ceño", "virgen escribana", "puso pie", "abrió puerta"},
		},
	}
	docs := make([]index.Document, 0, len(books))
	for _, b := range books {
		docs = append(docs, index.Document{
			ID: b.slug + ".epub", Slug: b.slug,
			Metadata: metadata.Metadata{
				Title: b.title, Authors: []string{b.author}, Description: template.HTML(b.description),
				Language: "es", Format: "EPUB",
			},
			AuthorsSlugs: []string{b.authorSlug}, TextRankPhrases: b.phrases,
		})
	}
	return docs
}

type relevanceCase struct {
	reference string
	// true means relevant; false means judged unrelated. Absent means unjudged.
	judgments map[string]bool
}

func similarityRelevanceCases() []relevanceCase {
	const matheson = "richard-matheson-el-increible-hombre-menguante"
	const asimov = "isaac-asimov-viaje-alucinante"
	const gulliver = "jonathan-swift-los-viajes-de-gulliver"
	const wells = "h-g-wells-el-hombre-invisible"
	const ward1 = "j-r-ward-amante-desatado"
	const ward2 = "j-r-ward-amante-liberada"
	return []relevanceCase{
		// Shared themes: shrinking/miniaturization, dramatic differences in
		// scale, and an individual transformed by a scientific event.
		{matheson, map[string]bool{asimov: true, gulliver: true, wells: true, ward1: false, ward2: false}},
		// The miniaturization connection is relevant in either direction;
		// leave other speculative fiction unjudged rather than guessing.
		{asimov, map[string]bool{matheson: true, ward1: false, ward2: false}},
		{gulliver, map[string]bool{matheson: true, ward1: false, ward2: false}},
		{wells, map[string]bool{matheson: true, ward1: false, ward2: false}},
	}
}

type relevanceMetrics struct {
	returned, judged, relevant, totalRelevant int
}

// relevanceAtK counts only judged results in precision; recall's denominator
// includes every relevant judgment, including documents not returned at all.
func relevanceAtK(ranking []string, judgments map[string]bool, k int) relevanceMetrics {
	var m relevanceMetrics
	for _, relevant := range judgments {
		if relevant {
			m.totalRelevant++
		}
	}
	for i, slug := range ranking {
		if i >= k {
			break
		}
		m.returned++
		if relevant, ok := judgments[slug]; ok {
			m.judged++
			if relevant {
				m.relevant++
			}
		}
	}
	return m
}

func (m relevanceMetrics) String() string {
	precision := "n/a"
	if m.judged != 0 {
		precision = fmt.Sprintf("%.2f", float64(m.relevant)/float64(m.judged))
	}
	return fmt.Sprintf("judged precision=%s (%d/%d), recall=%.2f (%d/%d), judged=%d/%d returned",
		precision, m.relevant, m.judged, float64(m.relevant)/float64(m.totalRelevant),
		m.relevant, m.totalRelevant, m.judged, m.returned)
}

// TestSimilarityRelevance reports the current quality rather than enshrining
// the known-bad ranking or failing CI until a quality target has been chosen.
// Run with: go test ./internal/index -run '^TestSimilarityRelevance$' -v
func TestSimilarityRelevance(t *testing.T) {
	docs := similarityRelevanceDocuments()
	bySlug := make(map[string]index.Document, len(docs))
	for _, doc := range docs {
		if _, exists := bySlug[doc.Slug]; exists {
			t.Fatalf("duplicate fixture slug %q", doc.Slug)
		}
		bySlug[doc.Slug] = doc
	}
	idx := newTextRankTestIndexWithConfig(t, docs, index.Config{MaxSimilarityPhrases: index.DefaultMaxSimilarityPhrases})
	for _, tc := range similarityRelevanceCases() {
		t.Run(tc.reference, func(t *testing.T) {
			reference, ok := bySlug[tc.reference]
			if !ok {
				t.Fatalf("missing reference %q", tc.reference)
			}
			positive, negative := 0, 0
			for slug, relevant := range tc.judgments {
				doc, ok := bySlug[slug]
				if !ok || slug == tc.reference || doc.AuthorsSlugs[0] == reference.AuthorsSlugs[0] {
					t.Fatalf("invalid or excluded judgment %q for %q", slug, tc.reference)
				}
				if relevant {
					positive++
				} else {
					negative++
				}
			}
			if positive == 0 || negative == 0 {
				t.Fatal("each reference needs both relevant and unrelated judgments")
			}
			fields := index.SearchFields{SimilarTo: tc.reference, SortBy: index.DefaultDocumentSortBy}
			started := time.Now()
			results, err := idx.Search(fields, 1, 4)
			if err != nil {
				t.Fatal(err)
			}
			elapsed := time.Since(started)
			ranking := slugsOf(results.Hits())
			seen := make(map[string]bool, len(ranking))
			for _, slug := range ranking {
				if _, exists := bySlug[slug]; !exists || slug == tc.reference || seen[slug] {
					t.Fatalf("invalid result %q in ranking %v", slug, ranking)
				}
				seen[slug] = true
			}
			t.Logf("current top 4: %v; %s; elapsed=%s", ranking, relevanceAtK(ranking, tc.judgments, 4), elapsed)
		})
	}
}

func TestRelevanceAtK(t *testing.T) {
	judgments := map[string]bool{"a": true, "b": true, "c": false}
	got := relevanceAtK([]string{"c", "unknown", "a", "b"}, judgments, 3)
	want := (relevanceMetrics{returned: 3, judged: 2, relevant: 1, totalRelevant: 2})
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	empty := relevanceAtK(nil, judgments, 4)
	if empty.String() != "judged precision=n/a (0/0), recall=0.00 (0/2), judged=0/0 returned" {
		t.Fatalf("unexpected empty-result metrics: %s", empty)
	}
}
