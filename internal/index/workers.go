package index

import "runtime"

const maxMetadataWorkers = 64

// ResolveMetadataWorkers picks the metadata extraction worker count used by indexing.
//
//	requested <= 0: automatic — runtime.NumCPU() clamped to [1, maxMetadataWorkers] (0 is default when unset).
//	requested == 1: sequential metadata extraction (single goroutine).
//	requested >= 2: bounded worker pool, capped at maxMetadataWorkers.
func ResolveMetadataWorkers(requested int) int {
	if requested <= 0 {
		n := runtime.NumCPU()
		if n < 1 {
			n = 1
		}
		if n > maxMetadataWorkers {
			n = maxMetadataWorkers
		}
		return n
	}
	if requested == 1 {
		return 1
	}
	if requested > maxMetadataWorkers {
		return maxMetadataWorkers
	}
	return requested
}
