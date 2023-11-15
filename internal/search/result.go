package search

import (
	"math"
)

// PaginatedResult holds the result of a search request, as well as some related metadata
type PaginatedResult[T any] struct {
	resultsPerPage int
	page           int
	totalPages     int
	hits           T
	totalHits      int
}

func NewPaginatedResult[T any](resultsPerPage, page, totalHits int, hits T) PaginatedResult[T] {
	return PaginatedResult[T]{
		resultsPerPage: resultsPerPage,
		page:           page,
		totalHits:      totalHits,
		hits:           hits,
	}
}

func (P PaginatedResult[T]) ResultsPerPage() int {
	return P.resultsPerPage
}

func (P PaginatedResult[T]) Page() int {
	return P.page
}

func (P PaginatedResult[T]) Hits() T {
	return P.hits
}

func (P PaginatedResult[T]) TotalHits() int {
	return P.totalHits
}

func (P PaginatedResult[T]) TotalPages() int {
	if P.resultsPerPage == 0 {
		return 0
	}

	if P.totalPages == 0 {
		P.totalPages = int(math.Ceil(float64(P.totalHits) / float64(P.resultsPerPage)))
	}
	return P.totalPages
}
