package model

import (
	"testing"

	"github.com/svera/coreander/v5/internal/index"
)

func TestMatchesSearchFieldsGoodReadsRatingRange(t *testing.T) {
	tests := []struct {
		name  string
		score float64
		from  float64
		to    float64
		want  bool
	}{
		{name: "inside range", score: 4.25, from: 4.2, to: 4.3, want: true},
		{name: "below minimum", score: 4.1, from: 4.2, to: 4.3},
		{name: "above maximum", score: 4.4, from: 4.2, to: 4.3},
		{name: "unrated", from: 4.2, to: 4.3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := AugmentedDocument{Document: index.Document{GoodReadsRatingValue: tt.score}}
			fields := index.SearchFields{GoodReadsRatingFrom: tt.from, GoodReadsRatingTo: tt.to}
			if got := matchesSearchFields(doc, fields, 0); got != tt.want {
				t.Errorf("matchesSearchFields() = %v, want %v", got, tt.want)
			}
		})
	}
}
