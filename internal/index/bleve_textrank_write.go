package index

import (
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/svera/coreander/v5/internal/metadata"
)

const (
	// backgroundPruneBatchSize is the batch size pruneCommonTextRankEntries uses
	// when triggered by maybePruneForLibraryChange, as opposed to the batchSize
	// EnrichTextRankKeywords's caller configures for the full startup pass.
	backgroundPruneBatchSize = 200

	// maxIndexedTextRankEntries hard-caps how many entries textRankKeywords ever
	// returns for each of TextRankPhrases/TextRankWords, regardless of
	// Config.MaxSimilarityPhrases (which only bounds how many phrases a
	// similarity query reads back, and can be set to 0 for "no cap"). Both fields
	// are indexed as multi-valued, term-vectored Bleve fields (one array position
	// per entry - see Document.TextRankPhrases), and a long or repetitive
	// document's uncapped word/phrase graph can produce thousands of entries; a
	// field value that large has been observed to corrupt zapx's location
	// encoding for that segment (a stray out-of-range field ID surfacing later in
	// unrelated Search calls, since corruption lives in the segment, not the
	// query). This limit is deliberately far above MaxSimilarityPhrases' own
	// default so it only ever trims the pathological tail, not the keywords any
	// real feature uses.
	maxIndexedTextRankEntries = 500
)

func (b *BleveIndexer) beginTextRankEnrichment(total int) {
	b.textRankEnrichProgress.begin(total)
}

func (b *BleveIndexer) endTextRankEnrichment() {
	b.textRankEnrichProgress.end()
}

func (b *BleveIndexer) recordTextRankEnrichmentProgress() {
	b.textRankEnrichProgress.record()
}

func (b *BleveIndexer) beginPruning(total int) {
	b.pruneProgress.begin(total)
}

func (b *BleveIndexer) endPruning() {
	b.pruneProgress.end()
}

func (b *BleveIndexer) recordPruningProgress(n int) {
	b.pruneProgress.recordN(n)
}

// runTextRankEnrichWorker drains textRankEnrichJobs for the indexer's
// lifetime, bounding how many enrichTextRankAndReindex calls run
// concurrently (see Config.TextRankEnrichWorkers).
func (b *BleveIndexer) runTextRankEnrichWorker() {
	for document := range b.textRankEnrichJobs {
		b.enrichTextRankAndReindex(document)
	}
}

// scheduleTextRankEnrichment queues document for the worker pool from a
// throwaway goroutine, so indexFile never blocks on the channel send.
func (b *BleveIndexer) scheduleTextRankEnrichment(document Document) {
	go func() {
		b.textRankEnrichJobs <- document
	}()
}

// enrichTextRankAndReindex runs TextRank analysis for a single, already
// -indexed document and persists the result. It mirrors what
// EnrichTextRankKeywords does in batches for the whole library, but for one
// document right after indexFile adds it, so a document uploaded through the
// web UI (or picked up by the file watcher) becomes searchable/browsable
// immediately and gets its keywords shortly after, without the caller
// waiting on TextRank.
func (b *BleveIndexer) enrichTextRankAndReindex(document Document) {
	enriched := b.rankDocument(document)
	b.documentsMu.Lock()
	err := b.documentsIdx.Index(enriched.ID, enriched)
	b.documentsMu.Unlock()
	if err != nil {
		log.Printf("Error indexing TextRank-enriched document %s: %s\n", enriched.ID, err)
	}
}

// rankTextFromContent runs TextRank analysis on textContent (already
// extracted from fullPath) and returns its phrases and single words, one per
// element, ready to store in Document.TextRankPhrases/Document.TextRankWords
// for full-text search. language is the document's already-known metadata
// language, passed through as a hint so the ranker doesn't have to reopen
// fullPath to derive it; fullPath is only used for logging. Returns nil, nil
// (logging any error non-fatally) if reader doesn't implement
// metadata.TextRanker or the analysis fails. If textContent has more than
// b.maxTextRankWords words (see Config.MaxTextRankWords), only its first
// b.maxTextRankWords are analyzed - this bounds TextRank's memory usage the
// same way skipping it outright would, while still producing keywords
// (based on the document's opening portion) rather than none at all.
func (b *BleveIndexer) rankTextFromContent(reader metadata.Reader, textContent, language, fullPath string) (phrases []string, words []string) {
	textRanker, ok := reader.(metadata.TextRanker)
	if !ok {
		return nil, nil
	}
	if fields := strings.Fields(textContent); b.maxTextRankWords > 0 && len(fields) > b.maxTextRankWords {
		log.Printf("Truncating text for TextRank analysis of %s: %d words exceeds max-textrank-words (%d)\n", fullPath, len(fields), b.maxTextRankWords)
		textContent = strings.Join(fields[:b.maxTextRankWords], " ")
	}
	result, err := textRanker.RankText(b.minOccurrenceRatio, textContent, language)
	if err != nil {
		log.Printf("Error ranking text for file %s: %s\n", fullPath, err)
		return nil, nil
	}
	if result == nil {
		return nil, nil
	}
	return textRankPhrasesAndWords(result)
}

// supportsTextRank reports whether the reader registered for fullPath's
// extension implements metadata.TextRanker at all (currently only EPUB).
// Used to mark documents in formats that can never be ranked as already
// "enriched", so EnrichTextRankKeywords never has to consider them.
func (b *BleveIndexer) supportsTextRank(fullPath string) bool {
	ext := strings.ToLower(filepath.Ext(fullPath))
	_, ok := b.reader[ext].(metadata.TextRanker)
	return ok
}

// documentsNeedingTextRankQuery builds the "not yet processed by
// EnrichTextRankKeywords" query shared by documentsNeedingTextRankCount and
// documentsNeedingTextRank (see Document.TextRankEnriched).
func documentsNeedingTextRankQuery() query.Query {
	notEnriched := bleve.NewBoolFieldQuery(false)
	notEnriched.SetField("TextRankEnriched")
	return notEnriched
}

// documentsNeedingTextRankCount reports how many documents still need
// TextRank analysis, without hydrating any of them, so
// EnrichTextRankKeywords can size its progress tracker up front while still
// fetching the documents themselves one batch at a time (see
// documentsNeedingTextRank).
func (b *BleveIndexer) documentsNeedingTextRankCount() (uint64, error) {
	searchReq := bleve.NewSearchRequestOptions(documentsNeedingTextRankQuery(), 0, 0, false)
	b.documentsMu.RLock()
	searchResult, err := b.documentsIdx.Search(searchReq)
	b.documentsMu.RUnlock()
	if err != nil {
		return 0, err
	}
	return searchResult.Total, nil
}

// documentsNeedingTextRank returns up to batchSize documents not yet
// processed by EnrichTextRankKeywords (see Document.TextRankEnriched),
// always querying From 0: EnrichTextRankKeywords persists each batch with
// TextRankEnriched set to true before requesting the next one, so already
// -processed documents drop out of this query's results on their own,
// without needing explicit pagination. This keeps at most batchSize
// documents' metadata in memory at once, rather than the whole library's
// pending set.
func (b *BleveIndexer) documentsNeedingTextRank(batchSize int) ([]Document, error) {
	searchReq := bleve.NewSearchRequestOptions(documentsNeedingTextRankQuery(), batchSize, 0, false)
	searchReq.Fields = []string{"*"}
	b.documentsMu.RLock()
	searchResult, err := b.documentsIdx.Search(searchReq)
	b.documentsMu.RUnlock()
	if err != nil {
		return nil, err
	}

	documents := make([]Document, 0, len(searchResult.Hits))
	for _, hit := range searchResult.Hits {
		documents = append(documents, hydrateDocument(hit))
	}
	return documents, nil
}

// rankDocument runs TextRank analysis for document (extracting its text via
// metadata.TextSource, if its reader supports it) and returns it with
// TextRankPhrases, TextRankWords, Words and TextRankEnriched set. Safe to call
// concurrently across documents, since it only reads
// from b and returns a modified copy.
func (b *BleveIndexer) rankDocument(document Document) Document {
	fullPath := filepath.Join(b.libraryPath, document.ID)
	ext := strings.ToLower(filepath.Ext(fullPath))
	reader := b.reader[ext]

	if textSource, ok := reader.(metadata.TextSource); ok {
		if text, err := textSource.Text(fullPath); err != nil {
			log.Printf("Error extracting text for %s: %s\n", fullPath, err)
		} else {
			document.TextRankPhrases, document.TextRankWords = b.rankTextFromContent(reader, text, document.Language, fullPath)
			document.Words = float64(len(strings.Fields(text)))
		}
	}
	document.TextRankEnriched = true
	return document
}

// rankDocuments runs rankDocument over docs, using up to workers goroutines
// in parallel (mirroring readMetadataForPaths), since TextRank analysis is
// CPU-bound and independent per document.
func (b *BleveIndexer) rankDocuments(docs []Document, workers int) []Document {
	out := make([]Document, len(docs))
	parallelFor(len(docs), workers, func(i int) {
		out[i] = b.rankDocument(docs[i])
		b.recordTextRankEnrichmentProgress()
	})
	return out
}

// EnrichTextRankKeywords finds documents indexed without TextRank analysis
// yet (see Document.TextRankEnriched) and computes+persists their TextRank
// keywords, one batch at a time so related-document recommendations and
// keyword search improve incrementally rather than all at once. Meant to run
// as a background pass after AddLibrary, which skips TextRank analysis
// itself so the library becomes searchable and browsable as fast as
// possible, even if recommendations aren't fully accurate until this
// finishes. workers controls parallelism the same way as AddLibrary's
// metadataWorkers, since TextRank analysis is as CPU-bound as metadata
// extraction and benefits just as much from running concurrently.
func (b *BleveIndexer) EnrichTextRankKeywords(batchSize, workers int) error {
	total, err := b.documentsNeedingTextRankCount()
	if err != nil {
		return err
	}

	if total > 0 {
		log.Printf("Enriching %d documents with TextRank keywords", total)
		b.beginTextRankEnrichment(int(total))

		// maxIterations bounds the loop below in case a document's batch.Index
		// call keeps failing (logged, non-fatal): such a document never gets
		// TextRankEnriched persisted as true, so it would otherwise keep
		// reappearing in documentsNeedingTextRank's results forever. A normal
		// run needs ceil(total/batchSize) iterations; this allows well beyond
		// that before giving up.
		maxIterations := int(total) + 10
		for iterations := 0; ; iterations++ {
			if iterations >= maxIterations {
				b.endTextRankEnrichment()
				return fmt.Errorf("TextRank enrichment did not finish after %d iterations, some documents may be stuck unenriched", iterations)
			}

			chunk, err := b.documentsNeedingTextRank(batchSize)
			if err != nil {
				b.endTextRankEnrichment()
				return err
			}
			if len(chunk) == 0 {
				break
			}

			enriched := b.rankDocuments(chunk, workers)

			batch := b.documentsIdx.NewBatch()
			for _, document := range enriched {
				if err := batch.Index(document.ID, document); err != nil {
					log.Printf("Error indexing enriched document %s: %s\n", document.ID, err)
				}
			}

			b.documentsMu.Lock()
			err = b.documentsIdx.Batch(batch)
			b.documentsMu.Unlock()
			if err != nil {
				b.endTextRankEnrichment()
				return err
			}
		}

		b.endTextRankEnrichment()
		log.Printf("TextRank enrichment finished")
	}

	// Skip pruning when nothing was enriched and the library hasn't changed
	// enough since the last prune - running it unconditionally on every
	// startup made an already-pruned, unchanged library unresponsive for no
	// benefit.
	if total > 0 || b.commonTextRankEntriesNeedPrune() {
		if err := b.pruneCommonTextRankEntries(batchSize); err != nil {
			return err
		}
	}

	return nil
}

// commonTextRankEntriesNeedPrune reports whether pruneCommonTextRankEntries
// hasn't run yet or the document count has changed by at least
// PruneChangeTriggerRatio since it last did (same threshold
// maybePruneForLibraryChange uses). Errors are treated as "needs pruning".
func (b *BleveIndexer) commonTextRankEntriesNeedPrune() bool {
	b.documentsMu.RLock()
	docCount, err := b.documentsIdx.DocCount()
	if err != nil {
		b.documentsMu.RUnlock()
		return true
	}
	stored, err := b.documentsIdx.GetInternal(internalDocCountAtLastPrune)
	b.documentsMu.RUnlock()
	if err != nil || len(stored) == 0 {
		return true
	}

	lastPruneCount, err := strconv.ParseUint(string(stored), 10, 64)
	if err != nil {
		return true
	}

	changed := int64(docCount) - int64(lastPruneCount)
	if changed < 0 {
		changed = -changed
	}
	return float64(changed) >= b.pruneChangeTriggerRatio*float64(lastPruneCount)
}

// maybePruneForLibraryChange compares the current document count against the
// count recorded after the last pruneCommonTextRankEntries pass and, once the
// difference exceeds pruneChangeTriggerRatio of that count, runs
// pruneCommonTextRankEntries in the background - mirroring
// enrichTextRankAndReindex's async pattern - so the upload/delete request or
// file watcher event that triggered it isn't blocked on a whole-library scan.
// Safe to call after every add/remove: a no-op call only costs a DocCount()
// and a GetInternal read.
func (b *BleveIndexer) maybePruneForLibraryChange() {
	b.documentsMu.RLock()
	docCount, err := b.documentsIdx.DocCount()
	if err != nil {
		b.documentsMu.RUnlock()
		return
	}
	stored, err := b.documentsIdx.GetInternal(internalDocCountAtLastPrune)
	b.documentsMu.RUnlock()
	if err != nil {
		return
	}

	// No prior prune recorded yet: EnrichTextRankKeywords will prune
	// unconditionally on its next run, so don't force one here too.
	if len(stored) == 0 {
		return
	}

	lastPruneCount, err := strconv.ParseUint(string(stored), 10, 64)
	if err != nil {
		return
	}

	changed := int64(docCount) - int64(lastPruneCount)
	if changed < 0 {
		changed = -changed
	}
	if float64(changed) < b.pruneChangeTriggerRatio*float64(lastPruneCount) {
		return
	}

	go func() {
		if err := b.pruneCommonTextRankEntries(backgroundPruneBatchSize); err != nil {
			log.Printf("Error pruning common TextRank entries: %s\n", err)
		}
	}()
}

// prunedTextRankEntries holds a document's post-pruning
// TextRankPhrases/TextRankWords.
type prunedTextRankEntries struct {
	phrases []string
	words   []string
}

// pruneCommonTextRankEntries removes TextRankPhrases/TextRankWords entries
// that appear in more than commonTextRankEntryRatio of the library from every
// document that has them, and persists the change. Run as the final step of
// EnrichTextRankKeywords rather than inside rankDocument, since "common" is a
// whole-library property that can only be measured after looking at every
// document's already-stored keywords, not something a single document's own
// TextRank pass can know about itself. Uses the exact strings already stored
// in TextRankPhrases/TextRankWords (rather than Bleve's field term
// dictionary) because those fields are tokenized for search - the term
// dictionary would report per-word frequency, not per-phrase frequency, for
// TextRankPhrases.
//
// Two passes over the index, both paginated batchSize documents at a time
// and requesting only TextRankPhrases/TextRankWords (not Fields "*") to
// avoid dragging along the rest of each document: the first tallies corpus
// frequency only (phraseDocCount/wordDocCount, sized by distinct entries,
// not document count); the second re-scans the same fields per batch,
// decides which of that batch's documents changed against the now-complete
// frequency tally, and immediately re-hydrates in full (unavoidable for
// reindexing) and rewrites just that batch. Earlier versions kept every
// document's entries (or every document that needed rewriting) in one
// unbounded map between the two passes - fine normally, but on the first
// prune ever, most documents can share at least one "common" entry, so
// "most of the library" and "the whole library" are the same order of
// magnitude; re-scanning instead of retaining is what keeps this bounded to
// batchSize documents even in that worst case.
func (b *BleveIndexer) pruneCommonTextRankEntries(batchSize int) error {
	b.documentsMu.RLock()
	docCount, err := b.documentsIdx.DocCount()
	b.documentsMu.RUnlock()
	if err != nil {
		return err
	}
	if docCount == 0 {
		return nil
	}

	log.Printf("Pruning common TextRank entries across %d documents", docCount)
	// *2: this function does two full passes over the index (counting, then
	// rewriting), each processing docCount documents.
	b.beginPruning(int(docCount) * 2)
	defer b.endPruning()

	phraseDocCount := make(map[string]int)
	wordDocCount := make(map[string]int)

	textRankFieldsSearch := func(from int) (*bleve.SearchResult, error) {
		req := bleve.NewSearchRequestOptions(bleve.NewMatchAllQuery(), batchSize, from, false)
		req.Fields = []string{"TextRankPhrases", "TextRankWords"}
		b.documentsMu.RLock()
		defer b.documentsMu.RUnlock()
		return b.documentsIdx.Search(req)
	}

	for from := 0; uint64(from) < docCount; from += batchSize {
		result, err := textRankFieldsSearch(from)
		if err != nil {
			return err
		}
		if len(result.Hits) == 0 {
			break
		}

		for _, hit := range result.Hits {
			// hydrateDocument assumes a full Fields "*" fetch (it
			// type-asserts every Metadata field unconditionally); this pass
			// only requests TextRankPhrases/TextRankWords, so read those two
			// directly via slicer instead.
			for _, phrase := range uniqueTextRankEntries(slicer(hit.Fields["TextRankPhrases"])) {
				phraseDocCount[phrase]++
			}
			for _, word := range uniqueTextRankEntries(slicer(hit.Fields["TextRankWords"])) {
				wordDocCount[word]++
			}
		}
		b.recordPruningProgress(len(result.Hits))
	}

	threshold := b.commonTextRankEntryRatio * float64(docCount)
	if threshold < float64(b.minCommonTextRankAbsoluteCount) {
		threshold = float64(b.minCommonTextRankAbsoluteCount)
	}

	pruned := 0
	// Once a library has stabilized, most prune runs (triggered by
	// maybePruneForLibraryChange after a handful of uploads/deletes) find
	// nothing newly common: skip the second full scan and all rewriting
	// entirely rather than re-reading every document just to discover that,
	// which is the common case in steady state.
	if !anyEntryAboveThreshold(phraseDocCount, threshold) && !anyEntryAboveThreshold(wordDocCount, threshold) {
		log.Printf("Pruning common TextRank entries: nothing exceeds the common-entry threshold, skipping rewrite")
	} else {
		pruned, err = b.rewriteCommonTextRankEntries(docCount, batchSize, phraseDocCount, wordDocCount, threshold, textRankFieldsSearch)
		if err != nil {
			return err
		}
	}

	log.Printf("Pruning common TextRank entries finished, %d documents updated", pruned)

	// Record the doc count this pass ran against so maybePruneForLibraryChange
	// can later tell how much the library has changed since.
	internalBatch := b.documentsIdx.NewBatch()
	internalBatch.SetInternal(internalDocCountAtLastPrune, []byte(strconv.FormatUint(docCount, 10)))
	b.documentsMu.Lock()
	err = b.documentsIdx.Batch(internalBatch)
	b.documentsMu.Unlock()
	return err
}

// anyEntryAboveThreshold reports whether any entry in counts occurs in more
// than threshold documents - i.e. whether pruning would actually change
// anything.
func anyEntryAboveThreshold(counts map[string]int, threshold float64) bool {
	for _, count := range counts {
		if float64(count) > threshold {
			return true
		}
	}
	return false
}

// rewriteCommonTextRankEntries is pruneCommonTextRankEntries' second pass:
// it re-scans the index (via search, the same paginated, lightweight-field
// query pass one used) to find which documents' TextRankPhrases/TextRankWords
// contain an entry above threshold, and rewrites just those, batchSize IDs at
// a time.
func (b *BleveIndexer) rewriteCommonTextRankEntries(docCount uint64, batchSize int, phraseDocCount, wordDocCount map[string]int, threshold float64, search func(from int) (*bleve.SearchResult, error)) (int, error) {
	pruned := 0
	for from := 0; uint64(from) < docCount; from += batchSize {
		result, err := search(from)
		if err != nil {
			return pruned, err
		}
		if len(result.Hits) == 0 {
			break
		}

		toPrune := make(map[string]prunedTextRankEntries, len(result.Hits))
		ids := make([]string, 0, len(result.Hits))
		for _, hit := range result.Hits {
			phrases := slicer(hit.Fields["TextRankPhrases"])
			words := slicer(hit.Fields["TextRankWords"])
			prunedPhrases := filterOutCommonTextRankEntries(phrases, phraseDocCount, threshold)
			prunedWords := filterOutCommonTextRankEntries(words, wordDocCount, threshold)
			if len(prunedPhrases) == len(phrases) && len(prunedWords) == len(words) {
				continue
			}
			toPrune[hit.ID] = prunedTextRankEntries{prunedPhrases, prunedWords}
			ids = append(ids, hit.ID)
		}

		if len(ids) == 0 {
			b.recordPruningProgress(len(result.Hits))
			continue
		}

		docSearchReq := bleve.NewSearchRequestOptions(bleve.NewDocIDQuery(ids), len(ids), 0, false)
		docSearchReq.Fields = []string{"*"}
		b.documentsMu.RLock()
		docSearchResult, err := b.documentsIdx.Search(docSearchReq)
		b.documentsMu.RUnlock()
		if err != nil {
			return pruned, err
		}

		batch := b.documentsIdx.NewBatch()
		for _, hit := range docSearchResult.Hits {
			document := hydrateDocument(hit)
			p, ok := toPrune[document.ID]
			if !ok {
				continue
			}
			document.TextRankPhrases = p.phrases
			document.TextRankWords = p.words
			if err := batch.Index(document.ID, document); err != nil {
				log.Printf("Error indexing document %s while pruning common TextRank entries: %s\n", document.ID, err)
				continue
			}
			pruned++
		}

		if batch.Size() > 0 {
			b.documentsMu.Lock()
			err = b.documentsIdx.Batch(batch)
			b.documentsMu.Unlock()
			if err != nil {
				return pruned, err
			}
		}
		b.recordPruningProgress(len(result.Hits))
	}

	return pruned, nil
}

// uniqueTextRankEntries de-duplicates a document's TextRankPhrases/
// TextRankWords before they're counted towards pruneCommonTextRankEntries'
// per-document-frequency maps, so a document listing the same entry twice
// (which rankDocument's own extraction shouldn't normally produce) can never
// inflate its corpus-wide count by more than one.
func uniqueTextRankEntries(entries []string) []string {
	if len(entries) == 0 {
		return entries
	}
	seen := make(map[string]struct{}, len(entries))
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		out = append(out, entry)
	}
	return out
}

// filterOutCommonTextRankEntries returns entries without any item whose
// corpus-wide document count (from docCount, keyed by entry) exceeds
// threshold, preserving the original (weight-descending) order.
func filterOutCommonTextRankEntries(entries []string, docCount map[string]int, threshold float64) []string {
	if len(entries) == 0 {
		return entries
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if float64(docCount[entry]) > threshold {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// textRankPhrasesAndWords turns a TextRankResult's phrases and single words into two
// slices - one string per phrase, one per single word - ready to store in
// Document.TextRankPhrases and Document.TextRankWords respectively, one per
// slice element (rather than a single flattened string) so each lands in its
// own array entry - see the Document.TextRankPhrases doc comment for why that
// separation matters. Each slice is ordered by descending Weight (TextRank's
// own, per-document-normalized importance score), so a caller that only uses
// a prefix (e.g. similarToQuery's Config.MaxSimilarityPhrases cap on
// TextRankPhrases) gets the most representative entries first rather than an
// arbitrary subset, and independently capped at maxIndexedTextRankEntries
// (see its own doc comment).
func textRankPhrasesAndWords(result *metadata.TextRankResult) (phrases []string, words []string) {
	phrases = weightedTextRankEntries(len(result.Phrases), func(i int) (string, float32) {
		p := result.Phrases[i]
		return p.Left + " " + p.Right, p.Weight
	})
	words = weightedTextRankEntries(len(result.SingleWords), func(i int) (string, float32) {
		w := result.SingleWords[i]
		return w.Word, w.Weight
	})
	return phrases, words
}

// weightedTextRankEntries builds a weight-sorted (descending), capped string
// slice out of n entries, each produced by at(i). Shared by
// textRankPhrasesAndWords for its Phrases/SingleWords slices, which come from
// different underlying types (rank.Phrase/rank.SingleWord) with no common
// interface.
func weightedTextRankEntries(n int, at func(i int) (text string, weight float32)) []string {
	if n == 0 {
		return nil
	}

	type weighted struct {
		text   string
		weight float32
	}
	entries := make([]weighted, n)
	for i := range n {
		text, weight := at(i)
		entries[i] = weighted{text, weight}
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].weight > entries[j].weight
	})

	if len(entries) > maxIndexedTextRankEntries {
		entries = entries[:maxIndexedTextRankEntries]
	}

	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.text
	}
	return out
}
