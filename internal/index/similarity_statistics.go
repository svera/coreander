package index

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"slices"
	"strconv"

	"github.com/blevesearch/bleve/v2/analysis"
	"github.com/blevesearch/bleve/v2/search/query"
)

var internalSimilarityPhraseStatistics = []byte("similarity_phrase_statistics")

const (
	similarityMaxPhraseFrequency  = 0.01
	similarityMinSharedPhrases    = 2
	similarityMaxSubjectFrequency = 0.20
)

type similarityStatistics struct {
	DocumentCount  uint64
	PhraseCounts   map[string]int
	SubjectCounts  map[string]int
	PruneRatio     float64
	PruneMinimum   int
	usefulSubjects []string
	subjectQuery   query.Query
}

func (b *BleveIndexer) loadSimilarityStatistics() (*similarityStatistics, error) {
	b.documentsMu.RLock()
	raw, err := b.documentsIdx.GetInternal(internalSimilarityPhraseStatistics)
	b.documentsMu.RUnlock()
	if err != nil {
		return nil, fmt.Errorf("read similarity statistics: %w", err)
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var stats similarityStatistics
	if err := json.Unmarshal(raw, &stats); err != nil {
		return nil, fmt.Errorf("decode similarity statistics: %w", err)
	}
	stats.prepareSubjectFrequencies()
	return &stats, nil
}

// Published snapshots are immutable. Requests reuse them until cleanup replaces
// them; missing or incomplete snapshots keep recommendations unavailable.
func (b *BleveIndexer) similarityStatistics() (*similarityStatistics, error) {
	b.similarityStatsMu.Lock()
	defer b.similarityStatsMu.Unlock()
	if b.similarityStatsLoaded {
		return b.similarityStats, nil
	}
	stats, err := b.loadSimilarityStatistics()
	if err != nil {
		return nil, err
	}
	if stats == nil || stats.SubjectCounts == nil {
		log.Printf("Similar documents unavailable until keyword clean-up collects phrase and subject statistics")
		b.similarityStatsLoaded = true
		return nil, nil
	}
	b.similarityStats = stats
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

func (s *similarityStatistics) prepareSubjectFrequencies() {
	threshold := math.Max(float64(DefaultMinCommonTextRankAbsoluteCount), float64(s.DocumentCount)*similarityMaxSubjectFrequency)
	s.usefulSubjects = nil
	for subject, count := range s.SubjectCounts {
		if subject != "" && count > 0 && float64(count) <= threshold {
			s.usefulSubjects = append(s.usefulSubjects, subject)
		}
	}
	slices.Sort(s.usefulSubjects)
	s.subjectQuery = usefulSimilaritySubjectsQuery(s.usefulSubjects)
}

func (b *BleveIndexer) publishSimilarityStatistics(stats *similarityStatistics) error {
	stats.prepareSubjectFrequencies()
	statsRaw, err := json.Marshal(stats)
	if err != nil {
		return fmt.Errorf("encode similarity statistics: %w", err)
	}
	// Publish the snapshot and maintenance marker atomically.
	internalBatch := b.documentsIdx.NewBatch()
	internalBatch.SetInternal(internalDocCountAtLastPrune, []byte(strconv.FormatUint(stats.DocumentCount, 10)))
	internalBatch.SetInternal(internalSimilarityPhraseStatistics, statsRaw)
	b.similarityStatsMu.Lock()
	defer b.similarityStatsMu.Unlock()
	b.documentsMu.Lock()
	err = b.documentsIdx.Batch(internalBatch)
	b.documentsMu.Unlock()
	if err == nil {
		b.similarityStats = stats
		b.similarityStatsLoaded = true
	}
	return err
}
