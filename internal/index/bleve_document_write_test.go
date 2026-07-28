package index

import (
	"reflect"
	"testing"

	"github.com/DavidBelicza/TextRank/v2/rank"
	"github.com/svera/coreander/v5/internal/metadata"
)

func TestTextRankKeywords(t *testing.T) {
	t.Run("returns one \"left right\" string per phrase, followed by one string per single word", func(t *testing.T) {
		result := &metadata.TextRankResult{
			Phrases: []rank.Phrase{
				{Left: "robert", Right: "oppenheimer"},
				{Left: "manhattan", Right: "project"},
			},
			SingleWords: []rank.SingleWord{
				{Word: "physics"},
				{Word: "chevalier"},
			},
		}

		got := textRankKeywords(result)
		want := []string{"robert oppenheimer", "manhattan project", "physics", "chevalier"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("expected %#v, got %#v", want, got)
		}
	})

	t.Run("no phrases or words yield a nil slice", func(t *testing.T) {
		got := textRankKeywords(&metadata.TextRankResult{})
		if got != nil {
			t.Errorf("expected nil, got %#v", got)
		}
	})
}
