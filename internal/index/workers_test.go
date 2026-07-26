package index

import "testing"

func TestResolveMetadataWorkers(t *testing.T) {
	t.Run("zero means automatic CPU parallelism", func(t *testing.T) {
		n := ResolveMetadataWorkers(0)
		if n < 1 || n > maxMetadataWorkers {
			t.Fatalf("expected clamped CPU-based count in [1, %d], got %d", maxMetadataWorkers, n)
		}
	})

	t.Run("explicit one is sequential", func(t *testing.T) {
		if got := ResolveMetadataWorkers(1); got != 1 {
			t.Fatalf("expected 1, got %d", got)
		}
	})

	t.Run("explicit value", func(t *testing.T) {
		if got := ResolveMetadataWorkers(4); got != 4 {
			t.Fatalf("expected 4, got %d", got)
		}
	})

	t.Run("explicit value capped at max", func(t *testing.T) {
		if got := ResolveMetadataWorkers(200); got != maxMetadataWorkers {
			t.Fatalf("expected %d, got %d", maxMetadataWorkers, got)
		}
	})
}
