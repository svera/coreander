package search

import "github.com/svera/coreander/v3/internal/metadata"

type Document struct {
	metadata.Metadata
	ID   string
	Slug string
}

// PaginatedResult holds the result of a search request, as well as some related metadata
type PaginatedResult struct {
	Page       int
	TotalPages int
	Hits       []Document
	TotalHits  int
}
