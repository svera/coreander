package index

import "testing"

func TestDefaultMaxTextRankWords(t *testing.T) {
	const mb = 1024 * 1024

	t.Run("undetectable RAM falls back to a conservative constant", func(t *testing.T) {
		if got := DefaultMaxTextRankWords(0, 4); got != fallbackMaxTextRankWords {
			t.Fatalf("expected %d, got %d", fallbackMaxTextRankWords, got)
		}
	})

	t.Run("more RAM yields a larger budget", func(t *testing.T) {
		small := DefaultMaxTextRankWords(512*mb, 1)
		large := DefaultMaxTextRankWords(4096*mb, 1)
		if large <= small {
			t.Fatalf("expected more RAM to yield a larger budget, got small=%d large=%d", small, large)
		}
	})

	t.Run("more concurrent workers shrink the per-worker budget", func(t *testing.T) {
		sequential := DefaultMaxTextRankWords(4096*mb, 1)
		parallel := DefaultMaxTextRankWords(4096*mb, 8)
		if parallel >= sequential {
			t.Fatalf("expected more workers to shrink the budget, got sequential=%d parallel=%d", sequential, parallel)
		}
	})

	t.Run("very little RAM or many workers still floors at the minimum", func(t *testing.T) {
		if got := DefaultMaxTextRankWords(64*mb, 64); got != minAutoMaxTextRankWords {
			t.Fatalf("expected floor of %d, got %d", minAutoMaxTextRankWords, got)
		}
	})

	t.Run("zero or negative workers are treated as 1", func(t *testing.T) {
		want := DefaultMaxTextRankWords(4096*mb, 1)
		if got := DefaultMaxTextRankWords(4096*mb, 0); got != want {
			t.Fatalf("expected workers=0 to behave like workers=1 (%d), got %d", want, got)
		}
		if got := DefaultMaxTextRankWords(4096*mb, -1); got != want {
			t.Fatalf("expected workers=-1 to behave like workers=1 (%d), got %d", want, got)
		}
	})
}
