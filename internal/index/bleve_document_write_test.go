package index

import (
	"testing"

	"github.com/DavidBelicza/TextRank/v2/rank"
	"github.com/svera/coreander/v5/internal/metadata"
)

func TestTextRankKeywords(t *testing.T) {
	t.Run("flattens phrases and single words into space-separated text", func(t *testing.T) {
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
		want := "robert oppenheimer manhattan project physics chevalier"
		if got != want {
			t.Errorf("expected %q, got %q", want, got)
		}
	})

	t.Run("empty phrases and words yield an empty string", func(t *testing.T) {
		got := textRankKeywords(&metadata.TextRankResult{})
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})
}
