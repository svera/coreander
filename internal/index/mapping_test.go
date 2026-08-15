package index

import (
	"testing"

	"github.com/blevesearch/bleve/v2/mapping"
	index "github.com/blevesearch/bleve_index_api"
)

// TestCreateDocumentsMappingUsesBM25Scoring guards against a real bug: BM25
// can only be turned on via IndexMappingImpl.ScoringModel, at the index
// level. Per-field FieldMapping.Similarity - previously set on several text
// fields here, presumably to opt into BM25 - is for KNN vector fields only
// and is silently ignored for text fields, so the index kept using classic
// TF-IDF scoring with no indication anything was wrong. TF-IDF's raw
// 1/sqrt(fieldLength) norm unfairly penalizes documents with a longer
// TextRankPhrases/TextRankWords list (e.g. richer/longer books yield more TextRank
// phrases) relative to shorter ones, regardless of actual relevance.
func TestCreateDocumentsMappingUsesBM25Scoring(t *testing.T) {
	got := CreateDocumentsMapping()
	m, ok := got.(*mapping.IndexMappingImpl)
	if !ok {
		t.Fatalf("expected CreateDocumentsMapping to return *mapping.IndexMappingImpl, got %T", got)
	}
	if m.ScoringModel != index.BM25Scoring {
		t.Errorf("expected ScoringModel to be %q, got %q", index.BM25Scoring, m.ScoringModel)
	}
}
