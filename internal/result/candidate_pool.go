package result

// CandidatePool records the size of a raw candidate pool a result was
// narrowed down from, the cap applied to it (e.g. BleveIndexer's
// maxSimilarityCandidates for "similar document" queries), and how many of
// the capped candidates actually matched (e.g. passed the similarity
// threshold), so callers can tell whether the cap plausibly left better
// matches out of consideration. The zero value means no cap was applied, and
// Capped reports false.
type CandidatePool struct {
	total   int
	matched int
	cap     int
}

// NewCandidatePool records total, the size of the raw candidate pool; matched,
// how many of the (at most cap) candidates considered actually matched; and
// cap, the limit that was applied to the pool.
func NewCandidatePool(total, matched, cap int) CandidatePool {
	return CandidatePool{total: total, matched: matched, cap: cap}
}

// Total returns the total size of the candidate pool, before any cap was
// applied.
func (c CandidatePool) Total() int {
	return c.total
}

// Cap returns the cap that was applied to the candidate pool.
func (c CandidatePool) Cap() int {
	return c.cap
}

// Capped reports whether Total is larger than Cap and every one of the
// capped candidates matched, i.e. whether the cap plausibly left better
// matches out of consideration. If fewer candidates matched than the cap
// allowed, the similarity threshold - not the cap - was the limiting factor,
// so widening the cap wouldn't have surfaced more matches.
func (c CandidatePool) Capped() bool {
	return c.cap > 0 && c.total > c.cap && c.matched >= c.cap
}

// SimilarityResult pairs a Paginated result with the candidate pool it was
// narrowed down from, for score-based "similar document" queries (see
// SearchFields.SimilarTo). Searches that aren't similarity-based just leave
// Candidates at its zero value, for which Capped reports false.
type SimilarityResult[T any] struct {
	Paginated[T]
	Candidates CandidatePool
}
