package webserver

import "github.com/svera/coreander/internal/metadata"

// Result holds the result of a search request, as well as some related metadata
type Result struct {
	Page       int
	TotalPages int
	Hits       []metadata.Metadata
	TotalHits  int
}

// Reader defines a set of reading operations over an index
type Reader interface {
	Search(keywords string, page, resultsPerPage int) (*Result, error)
	Count() (uint64, error)
	Close() error
}
