package model

import "github.com/svera/coreander/v4/internal/index"

// idxReader provides index-backed document lookups and word counts for repositories.
type idxReader interface {
	Documents(slugs []string) ([]index.Document, error)
	TotalWordCount(slugs []string) (float64, error)
}
