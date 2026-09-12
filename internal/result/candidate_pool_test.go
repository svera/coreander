package result_test

import (
	"testing"

	"github.com/svera/coreander/v5/internal/result"
)

// TestCandidatePoolCapped checks that Capped only reports true when the cap
// actually left candidates unconsidered: total exceeding cap isn't enough on
// its own, since if fewer candidates matched than the cap allowed, the
// similarity threshold - not the cap - was what limited the result, and
// widening the cap wouldn't have surfaced more matches.
func TestCandidatePoolCapped(t *testing.T) {
	cases := []struct {
		name    string
		total   int
		matched int
		cap     int
		want    bool
	}{
		{"no cap applied", 5126, 3, 0, false},
		{"total within cap", 150, 3, 200, false},
		{"total exceeds cap but few matched", 5126, 3, 200, false},
		{"total exceeds cap and every capped candidate matched", 5126, 200, 200, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pool := result.NewCandidatePool(c.total, c.matched, c.cap)
			if got := pool.Capped(); got != c.want {
				t.Errorf("Capped() = %v, want %v (total=%d, matched=%d, cap=%d)", got, c.want, c.total, c.matched, c.cap)
			}
		})
	}
}
