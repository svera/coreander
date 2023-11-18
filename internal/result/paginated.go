package result

import (
	"math"
)

// Paginated holds the result of a search request, as well as some related metadata
type Paginated[T any] struct {
	maxResultsPerPage int
	page              int
	totalPages        int
	hits              T
	totalHits         int
}

func NewPaginated[T any](maxResultsPerPage, page, totalHits int, hits T) Paginated[T] {
	return Paginated[T]{
		maxResultsPerPage: maxResultsPerPage,
		page:              page,
		totalHits:         totalHits,
		hits:              hits,
	}
}

func (P Paginated[T]) MaxResultsPerPage() int {
	return P.maxResultsPerPage
}

func (P Paginated[T]) Page() int {
	return P.page
}

func (P Paginated[T]) Hits() T {
	return P.hits
}

func (P Paginated[T]) TotalHits() int {
	return P.totalHits
}

func (P Paginated[T]) TotalPages() int {
	if P.totalPages != 0 {
		return P.totalPages
	}

	if P.maxResultsPerPage == 0 {
		return 0
	}

	P.totalPages = int(math.Ceil(float64(P.totalHits) / float64(P.maxResultsPerPage)))
	return P.totalPages
}
