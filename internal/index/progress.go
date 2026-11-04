package index

import (
	"fmt"
	"math"
	"time"

	"github.com/blevesearch/bleve/v2"
)

type ProgressKind string

const (
	ProgressDocuments   ProgressKind = "documents"
	ProgressAuthors     ProgressKind = "authors"
	ProgressTextRank    ProgressKind = "textrank"
	ProgressPruning     ProgressKind = "pruning"
	ProgressDataSources ProgressKind = "data-sources"
)

type Progress struct {
	Kind          ProgressKind
	InProgress    bool
	RemainingTime time.Duration
	Percentage    float64
	Processed     uint64
	Total         uint64
	Source        string
	Name          string
}

// BackgroundProgress includes concurrent tasks and persisted pending provider lookups.
func (b *BleveIndexer) BackgroundProgress() ([]Progress, error) {
	var progress []Progress
	for _, task := range []struct {
		kind    ProgressKind
		tracker *progressTracker
	}{
		{ProgressDocuments, &b.indexProgress},
		{ProgressAuthors, &b.authorEnrichProgress},
		{ProgressTextRank, &b.textRankEnrichProgress},
		{ProgressPruning, &b.pruneProgress},
	} {
		if start := task.tracker.startNanos.Load(); start != 0 {
			progress = append(progress, b.progressFrom(task.kind, start, task.tracker.processed.Load(), task.tracker.total.Load()))
		}
	}
	if len(b.documentEnrichers) == 0 {
		return progress, nil
	}
	b.documentsMu.RLock()
	defer b.documentsMu.RUnlock()
	total, err := b.documentsIdx.DocCount()
	if err != nil {
		return nil, fmt.Errorf("counting documents for enrichment progress: %w", err)
	}
	for i, provider := range b.documentEnrichers {
		query := bleve.NewTermQuery(provider.Source())
		query.SetField("EnrichedDataSources")
		result, err := b.documentsIdx.Search(bleve.NewSearchRequestOptions(query, 0, 0, false))
		if err != nil {
			return nil, fmt.Errorf("reading %s enrichment progress: %w", provider.Source(), err)
		}
		if result.Total < total {
			// Do not round pending work up to 100%.
			progress = append(progress, Progress{
				Kind: ProgressDataSources, InProgress: true, Source: provider.Source(), Name: provider.Name(),
				Processed: result.Total, Total: total,
				Percentage:    math.Floor(100 * float64(result.Total) / float64(total)),
				RemainingTime: b.documentEnrichTracker(i).remainingTime(total - result.Total),
			})
		}
	}
	return progress, nil
}
