package index

import (
	"encoding/json"
	"fmt"
	"log"
	"math"

	"github.com/blevesearch/bleve/v2/analysis"
)

var internalSimilarityPhraseStatistics = []byte("similarity_phrase_statistics")

const (
	similarityMaxPhraseFrequency = 0.01
	similarityMinSharedPhrases   = 2
)

type similarityPhraseStatistics struct {
	DocumentCount  uint64
	PhraseCounts   map[string]int
	SubjectCounts  map[string]int
	PruneRatio     float64
	PruneMinimum   int
	commonSubjects []string
}

// Snapshots are immutable after publication. Missing statistics fail closed
// until background cleanup has collected them; requests never scan the library.
func (b *BleveIndexer) similarityStatistics() (*similarityPhraseStatistics, error) {
	b.similarityStatsMu.Lock()
	defer b.similarityStatsMu.Unlock()
	if b.similarityStatsLoaded {
		return b.similarityStats, nil
	}
	b.documentsMu.RLock()
	raw, err := b.documentsIdx.GetInternal(internalSimilarityPhraseStatistics)
	b.documentsMu.RUnlock()
	if err != nil {
		return nil, fmt.Errorf("read similarity phrase statistics: %w", err)
	}
	if len(raw) == 0 {
		log.Printf("Similar documents unavailable until keyword clean-up collects phrase statistics")
		b.similarityStatsLoaded = true
		return nil, nil
	}
	var stats similarityPhraseStatistics
	if err := json.Unmarshal(raw, &stats); err != nil {
		return nil, fmt.Errorf("decode similarity phrase statistics: %w", err)
	}
	if stats.SubjectCounts == nil {
		log.Printf("Similar documents unavailable until keyword clean-up collects subject statistics")
		b.similarityStatsLoaded = true
		return nil, nil
	}
	stats.prepareSubjectFrequencies()
	b.similarityStats = &stats
	b.similarityStatsLoaded = true
	return b.similarityStats, nil
}

func similarityPhraseKey(phrase string, analyzer analysis.Analyzer) string {
	tokens := analyzer.Analyze([]byte(phrase))
	if len(tokens) != 2 {
		return ""
	}
	return string(tokens[0].Term) + " " + string(tokens[1].Term)
}

func (b *BleveIndexer) distinctiveSimilarityPhrases(doc Document, stats *similarityPhraseStatistics) ([]string, error) {
	if stats == nil {
		return nil, nil
	}
	b.documentsMu.RLock()
	analyzer := b.documentsIdx.Mapping().AnalyzerNamed(defaultAnalyzer)
	b.documentsMu.RUnlock()
	if analyzer == nil {
		return nil, fmt.Errorf("missing similarity phrase analyzer %q", defaultAnalyzer)
	}
	threshold := similarityPhraseThreshold(stats.DocumentCount)
	phrases := make([]string, 0, len(doc.TextRankPhrases))
	seen := make(map[string]bool)
	for _, phrase := range doc.TextRankPhrases {
		key := similarityPhraseKey(phrase, analyzer)
		count, known := stats.PhraseCounts[key]
		if key == "" || seen[key] || !known || count < similarityMinSharedPhrases || float64(count) > threshold {
			continue
		}
		seen[key] = true
		phrases = append(phrases, phrase)
		if b.maxSimilarityPhrases > 0 && len(phrases) == b.maxSimilarityPhrases {
			break
		}
	}
	return phrases, nil
}

func similarityPhraseThreshold(docCount uint64) float64 {
	return math.Max(float64(DefaultMinCommonTextRankAbsoluteCount), float64(docCount)*similarityMaxPhraseFrequency)
}

func eligibleSimilarityPhraseCounts(counts map[string]int, docCount uint64) map[string]int {
	threshold := similarityPhraseThreshold(docCount)
	for phrase, count := range counts {
		if phrase == "" || count < similarityMinSharedPhrases || float64(count) > threshold {
			delete(counts, phrase)
		}
	}
	return counts
}
