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

// Paginate wraps hits in pagination metadata. When len(hits) equals totalHits, hits is treated
// as the full result set and sliced for the requested page. Otherwise hits is assumed to already
// contain the page window (for example from a database or Bleve query).
func Paginate[T any](maxResultsPerPage, page, totalHits int, hits []T) Paginated[[]T] {
	if page < 1 {
		page = 1
	}
	if totalHits == 0 {
		return NewPaginated(maxResultsPerPage, page, 0, []T{})
	}

	start := (page - 1) * maxResultsPerPage
	if start >= totalHits {
		return NewPaginated(maxResultsPerPage, page, totalHits, []T{})
	}

	if len(hits) == totalHits {
		end := start + maxResultsPerPage
		if end > totalHits {
			end = totalHits
		}
		return NewPaginated(maxResultsPerPage, page, totalHits, hits[start:end])
	}

	return NewPaginated(maxResultsPerPage, page, totalHits, hits)
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
