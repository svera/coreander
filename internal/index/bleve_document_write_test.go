package index

import (
	"reflect"
	"testing"

	"github.com/DavidBelicza/TextRank/v2/rank"
	"github.com/svera/coreander/v5/internal/metadata"
)

func TestTextRankPhrasesAndWords(t *testing.T) {
	t.Run("returns one \"left right\" string per phrase in TextRankPhrases, and one string per single word in TextRankWords", func(t *testing.T) {
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

		gotPhrases, gotWords := textRankPhrasesAndWords(result)
		wantPhrases := []string{"robert oppenheimer", "manhattan project"}
		wantWords := []string{"physics", "chevalier"}
		if !reflect.DeepEqual(gotPhrases, wantPhrases) {
			t.Errorf("expected phrases %#v, got %#v", wantPhrases, gotPhrases)
		}
		if !reflect.DeepEqual(gotWords, wantWords) {
			t.Errorf("expected words %#v, got %#v", wantWords, gotWords)
		}
	})

	t.Run("no phrases or words yield nil slices", func(t *testing.T) {
		gotPhrases, gotWords := textRankPhrasesAndWords(&metadata.TextRankResult{})
		if gotPhrases != nil {
			t.Errorf("expected nil phrases, got %#v", gotPhrases)
		}
		if gotWords != nil {
			t.Errorf("expected nil words, got %#v", gotWords)
		}
	})
}
