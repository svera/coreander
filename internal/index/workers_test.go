package index

import "testing"

func TestResolveMetadataWorkers(t *testing.T) {
	t.Run("auto uses at least one", func(t *testing.T) {
		n := ResolveMetadataWorkers(0, false)
		if n < 1 {
			t.Fatalf("expected >= 1, got %d", n)
		}
	})

	t.Run("manual zero is sequential", func(t *testing.T) {
		if got := ResolveMetadataWorkers(0, true); got != 1 {
			t.Fatalf("expected 1, got %d", got)
		}
	})

	t.Run("manual respects value", func(t *testing.T) {
		if got := ResolveMetadataWorkers(4, true); got != 4 {
			t.Fatalf("expected 4, got %d", got)
		}
	})

	t.Run("manual caps at max", func(t *testing.T) {
		if got := ResolveMetadataWorkers(200, true); got != maxMetadataWorkers {
			t.Fatalf("expected %d, got %d", maxMetadataWorkers, got)
		}
	})
}
