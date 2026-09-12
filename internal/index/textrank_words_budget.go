package index

const (
	// textRankBytesPerWord is a rough upper-bound estimate of how many bytes
	// TextRank's co-occurrence graph uses per word processed. Derived from
	// the previously documented guidance that ~350000 words is a reasonable
	// cap on a 512MB system: (512MB * textRankMemoryBudgetFraction) / 350000
	// words.
	textRankBytesPerWord = 384
	// textRankMemoryBudgetFraction is the fraction of total system RAM set
	// aside for TextRank's word budget, leaving headroom for the OS, the
	// rest of the app, and, since EnrichTextRankKeywords can rank multiple
	// documents concurrently (see parallelFor), every other in-flight
	// worker's own graph.
	textRankMemoryBudgetFraction = 0.25
	// minAutoMaxTextRankWords floors the auto-computed cap so a system with
	// very little (or many concurrent workers dividing little) RAM still
	// gets a usable, non-zero budget instead of one so small it produces
	// meaningless keywords.
	minAutoMaxTextRankWords = 20000
	// fallbackMaxTextRankWords is used when total system RAM can't be
	// determined, so an undetectable amount of RAM still yields a
	// conservative cap rather than disabling it outright.
	fallbackMaxTextRankWords = 200000
)

// DefaultMaxTextRankWords computes a safe value for Config.MaxTextRankWords
// from the amount of system RAM available and how many TextRank rankings can
// run concurrently, so a single long or repetitive document's TextRank graph
// can't exhaust memory on a memory-constrained system without requiring an
// explicit, manually-tuned flag. totalRAMBytes is the host's total RAM in
// bytes (0 if it couldn't be determined); workers is the number of
// concurrent TextRank rankings EnrichTextRankKeywords may run at once (see
// parallelFor); values below 1 are treated as 1.
func DefaultMaxTextRankWords(totalRAMBytes uint64, workers int) int {
	if workers < 1 {
		workers = 1
	}
	if totalRAMBytes == 0 {
		return fallbackMaxTextRankWords
	}
	budget := float64(totalRAMBytes) * textRankMemoryBudgetFraction / float64(workers)
	words := int(budget / textRankBytesPerWord)
	if words < minAutoMaxTextRankWords {
		words = minAutoMaxTextRankWords
	}
	return words
}
