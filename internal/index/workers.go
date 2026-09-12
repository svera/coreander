package index

import (
	"runtime"
	"sync"
)

const maxMetadataWorkers = 64

// parallelFor calls work(i) for every i in [0, n), using up to workers
// goroutines in parallel, and blocks until all calls have returned. work is
// responsible for writing its own result (e.g. into a slice captured by the
// closure) and for reporting its own progress, since those differ per caller
// (see readMetadataForPaths and rankDocuments). Shared so both of those don't
// have to duplicate the same sequential-vs-worker-pool branching.
func parallelFor(n, workers int, work func(i int)) {
	if workers <= 1 {
		for i := 0; i < n; i++ {
			work(i)
		}
		return
	}
	if workers > maxMetadataWorkers {
		workers = maxMetadataWorkers
	}
	if workers > n {
		workers = n
	}

	jobs := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				work(i)
			}
		}()
	}
	for i := 0; i < n; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
}

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
