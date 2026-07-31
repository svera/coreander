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
	candidatesTotal   int
	candidatesCap     int
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

// WithCandidates records the size of the raw candidate pool a result was
// narrowed down from, and the cap applied to it (e.g. BleveIndexer's
// maxSimilarityCandidates for "similar document" queries), so callers can
// tell whether some candidates were left out of consideration before
// pruning/pagination even ran. Not all searches have such a cap; callers that
// never call this get candidatesTotal/candidatesCap at their zero value, and
// CandidatesCapped reports false.
func (P Paginated[T]) WithCandidates(candidatesTotal, candidatesCap int) Paginated[T] {
	P.candidatesTotal = candidatesTotal
	P.candidatesCap = candidatesCap
	return P
}

// CandidatesTotal returns the total size of the candidate pool passed to
// WithCandidates, before any cap was applied.
func (P Paginated[T]) CandidatesTotal() int {
	return P.candidatesTotal
}

// CandidatesCap returns the cap passed to WithCandidates that was applied to
// the candidate pool.
func (P Paginated[T]) CandidatesCap() int {
	return P.candidatesCap
}

// CandidatesCapped reports whether the candidate pool recorded via
// WithCandidates was larger than the cap applied to it, i.e. whether some
// candidates were left out of consideration.
func (P Paginated[T]) CandidatesCapped() bool {
	return P.candidatesCap > 0 && P.candidatesTotal > P.candidatesCap
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
