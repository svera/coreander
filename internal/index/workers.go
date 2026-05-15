package index

import "runtime"

const maxMetadataWorkers = 64

// ResolveMetadataWorkers picks the metadata extraction worker count.
// When manuallySet is false, it uses runtime.NumCPU() (clamped to [1, maxMetadataWorkers]).
// When manuallySet is true, values 0 or 1 mean sequential; larger values are capped at maxMetadataWorkers.
func ResolveMetadataWorkers(requested int, manuallySet bool) int {
	if !manuallySet {
		n := runtime.NumCPU()
		if n < 1 {
			n = 1
		}
		if n > maxMetadataWorkers {
			n = maxMetadataWorkers
		}
		return n
	}
	if requested <= 1 {
		return 1
	}
	if requested > maxMetadataWorkers {
		return maxMetadataWorkers
	}
	return requested
}
