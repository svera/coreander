package index

import "runtime"

const maxMetadataWorkers = 64

// ResolveMetadataWorkers picks the metadata extraction worker count.
// When requested is nil, it uses runtime.NumCPU() (clamped to [1, maxMetadataWorkers]).
// When set, values 0 or 1 mean sequential; larger values are capped at maxMetadataWorkers.
func ResolveMetadataWorkers(requested *int) int {
	if requested == nil {
		n := runtime.NumCPU()
		if n < 1 {
			n = 1
		}
		if n > maxMetadataWorkers {
			n = maxMetadataWorkers
		}
		return n
	}
	if *requested <= 1 {
		return 1
	}
	if *requested > maxMetadataWorkers {
		return maxMetadataWorkers
	}
	return *requested
}
