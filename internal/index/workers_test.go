package index

import "testing"

func TestResolveMetadataWorkers(t *testing.T) {
	t.Run("nil uses at least one", func(t *testing.T) {
		n := ResolveMetadataWorkers(nil)
		if n < 1 {
			t.Fatalf("expected >= 1, got %d", n)
		}
	})

	t.Run("explicit zero is sequential", func(t *testing.T) {
		zero := 0
		if got := ResolveMetadataWorkers(&zero); got != 1 {
			t.Fatalf("expected 1, got %d", got)
		}
	})

	t.Run("explicit value", func(t *testing.T) {
		four := 4
		if got := ResolveMetadataWorkers(&four); got != 4 {
			t.Fatalf("expected 4, got %d", got)
		}
	})

	t.Run("explicit value capped at max", func(t *testing.T) {
		n := 200
		if got := ResolveMetadataWorkers(&n); got != maxMetadataWorkers {
			t.Fatalf("expected %d, got %d", maxMetadataWorkers, got)
		}
	})
}
