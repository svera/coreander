package search

import (
	"math"

	"github.com/svera/coreander/v4/internal/metadata"
)

type Document struct {
	metadata.Metadata
	ID          string
	Slug        string
	Highlighted bool
}

// PaginatedResult holds the result of a search request, as well as some related metadata
type PaginatedResult struct {
	Page       int
	TotalPages int
	Hits       []Document
	TotalHits  int
}

func CalculateTotalPages(total, resultsPerPage uint64) int {
	return int(math.Ceil(float64(total) / float64(resultsPerPage)))
}
