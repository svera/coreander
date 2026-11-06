package index

import "time"

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
	Name          string
}
