package index

import "time"

type ProgressKind string

const (
	ProgressDocuments ProgressKind = "documents"
	ProgressAuthors   ProgressKind = "authors"
	ProgressTextRank  ProgressKind = "textrank"
)

type Progress struct {
	Kind          ProgressKind
	InProgress    bool
	RemainingTime time.Duration
	Percentage    float64
}
