package index

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/blevesearch/bleve/v2"
	index "github.com/blevesearch/bleve_index_api"
	"github.com/gosimple/slug"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/metadata"
)

// documentSlugCollisionPattern matches slugs like "title--2" used for disambiguation.
var documentSlugCollisionPattern = regexp.MustCompile(`^[a-zA-Z0-9\-]+(--)[0-9]+$`)

func (b *BleveIndexer) IndexingProgress() (Progress, error) {
	if b.indexStartNanos.Load() != 0 {
		return b.progressFrom(ProgressDocuments, b.indexStartNanos.Load(), b.indexedEntries.Load(), b.indexTotalEntries.Load()), nil
	}
	if b.authorEnrichStartNanos.Load() != 0 {
		return b.progressFrom(ProgressAuthors, b.authorEnrichStartNanos.Load(), b.authorEnrichProcessed.Load(), b.authorEnrichTotalEntries.Load()), nil
	}
	if b.textRankEnrichStartNanos.Load() != 0 {
		return b.progressFrom(ProgressTextRank, b.textRankEnrichStartNanos.Load(), b.textRankEnrichProcessed.Load(), b.textRankEnrichTotalEntries.Load()), nil
	}
	return Progress{}, nil
}

func (b *BleveIndexer) progressFrom(kind ProgressKind, startNanos int64, processed, total uint64) Progress {
	progress := Progress{Kind: kind, InProgress: true}
	if total > 0 {
		progress.Percentage = math.Round(100 * float64(processed) / float64(total))
		if progress.Percentage > 100 {
			progress.Percentage = 100
		}
	}
	if processed > 0 && processed < total {
		elapsed := float64(time.Now().UnixNano()) - float64(startNanos)
		progress.RemainingTime = time.Duration(elapsed * float64(total-processed) / float64(processed))
	}
	return progress
}

func (b *BleveIndexer) beginIndexing() {
	b.indexStartNanos.Store(time.Now().UnixNano())
	b.indexedEntries.Store(0)
	b.indexTotalEntries.Store(0)
}

func (b *BleveIndexer) endIndexing() {
	b.indexStartNanos.Store(0)
	b.indexedEntries.Store(0)
	b.indexTotalEntries.Store(0)
}

func (b *BleveIndexer) beginTextRankEnrichment(total int) {
	b.textRankEnrichStartNanos.Store(time.Now().UnixNano())
	b.textRankEnrichProcessed.Store(0)
	b.textRankEnrichTotalEntries.Store(uint64(total))
}

func (b *BleveIndexer) endTextRankEnrichment() {
	b.textRankEnrichStartNanos.Store(0)
	b.textRankEnrichProcessed.Store(0)
	b.textRankEnrichTotalEntries.Store(0)
}

func (b *BleveIndexer) recordTextRankEnrichmentProgress() {
	b.textRankEnrichProcessed.Add(1)
}

// NewFile writes the given contents to the library as fileName, indexes it, and returns the document slug.
func (b *BleveIndexer) NewFile(fileName string, contents []byte) (string, error) {
	fullPath := filepath.Join(b.libraryPath, fileName)
	f, err := b.fs.Create(fullPath)
	if err != nil {
		return "", fmt.Errorf("creating file %s: %w", fullPath, err)
	}
	_, err = f.Write(contents)
	if err != nil {
		f.Close()
		_ = b.fs.Remove(fullPath)
		return "", fmt.Errorf("writing file %s: %w", fullPath, err)
	}
	if err := f.Close(); err != nil {
		_ = b.fs.Remove(fullPath)
		return "", fmt.Errorf("closing file %s: %w", fullPath, err)
	}
	slug, err := b.indexFile(fullPath)
	if err != nil {
		_ = b.fs.Remove(fullPath)
		return "", err
	}
	return slug, nil
}

// indexFile adds a file to the index
func (b *BleveIndexer) indexFile(file string) (string, error) {
	ext := strings.ToLower(filepath.Ext(file))
	if _, ok := b.reader[ext]; !ok {
		return "", fmt.Errorf("file extension %s not supported", ext)
	}
	meta, phrases, words, err := b.metadataAndKeywordsFor(ext, file)
	if err != nil {
		return "", fmt.Errorf("error extracting metadata from file %s: %s", file, err)
	}

	document := b.createDocument(meta, file, nil, nil)
	document.AddedOn = time.Now().UTC()
	document.TextRankPhrases = phrases
	document.TextRankWords = words

	b.documentsMu.Lock()
	err = b.documentsIdx.Index(document.ID, document)
	b.documentsMu.Unlock()
	if err != nil {
		return "", fmt.Errorf("error indexing file %s: %s", file, err)
	}

	if err := b.incrementAuthorCounts(document.Authors, document.AuthorsSlugs); err != nil {
		return document.Slug, err
	}
	if err := b.incrementAuthorCounts(document.Illustrators, document.IllustratorsSlugs); err != nil {
		return document.Slug, err
	}

	return document.Slug, nil
}

// removeFile removes a file from the index
func (b *BleveIndexer) removeFile(file string) error {
	id := b.id(file)
	document, err := b.documentByIndexID(id)
	if err != nil {
		return err
	}
	if document.ID != "" {
		return b.deleteDocumentFromIndex(document)
	}
	b.documentsMu.Lock()
	defer b.documentsMu.Unlock()
	return b.documentsIdx.Delete(id)
}

// DeleteDocument removes the document identified by slug from the index and deletes its file from the filesystem.
func (b *BleveIndexer) DeleteDocument(slug string) error {
	document, err := b.Document(slug)
	if err != nil {
		return err
	}
	if document.Slug == "" {
		return ErrDocumentNotFound
	}
	if err := b.deleteDocumentFromIndex(document); err != nil {
		return err
	}
	fullPath := filepath.Join(b.libraryPath, document.ID)
	if err := b.fs.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		log.Printf("error removing file %s: %s\n", fullPath, err.Error())
	}
	return nil
}

func (b *BleveIndexer) deleteDocumentFromIndex(document Document) error {
	b.documentsMu.Lock()
	err := b.documentsIdx.Delete(document.ID)
	b.documentsMu.Unlock()
	if err != nil {
		return err
	}
	for _, authorSlug := range authorSlugsFromDocument(document) {
		author, err := b.Author(authorSlug, "")
		if err != nil {
			return err
		}
		if author.Slug == "" {
			continue
		}
		b.authorsMu.Lock()
		if author.DocumentCount <= 1 {
			err = b.authorsIdx.Delete(authorSlug)
		} else {
			author.DocumentCount--
			err = b.authorsIdx.Index(authorSlug, author)
		}
		b.authorsMu.Unlock()
		if err != nil {
			return err
		}
	}
	return nil
}

func authorSlugsFromDocument(document Document) []string {
	slugs := make([]string, 0, len(document.AuthorsSlugs)+len(document.IllustratorsSlugs))
	for _, authorSlug := range append(document.AuthorsSlugs, document.IllustratorsSlugs...) {
		if authorSlug == "" || slices.Contains(slugs, authorSlug) {
			continue
		}
		slugs = append(slugs, authorSlug)
	}
	return slugs
}

// AddLibrary scans <libraryPath> for documents and adds them to the index in batches of <batchSize> if they
// haven't been previously indexed or if <forceIndexing> is true.
// metadataWorkers controls parallel metadata extraction after CLI resolution: 1 is fully sequential; values
// greater than 1 use a bounded worker pool while Bleve batching and slug resolution stay on a single goroutine.
//
// This is intentionally a fast, metadata-only pass: it does not run TextRank analysis, so a document
// becomes searchable as soon as its batch commits rather than waiting on the whole library's worth of
// ranking. Each batchSize-sized chunk of pending paths is extracted and committed before moving on to the
// next, so documents appear incrementally instead of only after every pending file has been processed.
// EnrichTextRankKeywords fills in TextRank keywords afterward, in the background.
func (b *BleveIndexer) AddLibrary(batchSize int, forceIndexing bool, metadataWorkers int) error {
	b.beginIndexing()

	pending, languages, err := b.collectPendingLibraryPaths(forceIndexing)
	if err != nil {
		b.endIndexing()
		return err
	}
	b.indexTotalEntries.Store(b.indexedEntries.Load() + uint64(len(pending)))
	slices.Sort(pending)

	documentsSeen := make(map[string]Document, len(pending))

	for chunkStart := 0; chunkStart < len(pending); chunkStart += batchSize {
		chunk := pending[chunkStart:min(chunkStart+batchSize, len(pending))]
		metaJobs := b.readMetadataForPaths(chunk, metadataWorkers)

		batch := b.documentsIdx.NewBatch()
		batchSlugs := make(map[string]struct{}, len(chunk))

		for _, job := range metaJobs {
			if job.err != nil {
				log.Printf("Error extracting metadata from file %s: %s\n", job.path, job.err)
				continue
			}

			document := b.createDocument(job.meta, job.path, batchSlugs, documentsSeen)
			batchSlugs[document.Slug] = struct{}{}
			languages = addLanguage(job.meta.Language, languages)
			document.AddedOn = time.Time{}
			// Formats that can never support TextRank (e.g. PDF) are marked
			// enriched immediately, so EnrichTextRankKeywords never has to
			// consider them.
			document.TextRankEnriched = !b.supportsTextRank(job.path)

			if err = batch.Index(document.ID, document); err != nil {
				log.Printf("Error indexing file %s: %s\n", job.path, err)
				continue
			}
		}

		b.documentsMu.Lock()
		err = b.documentsIdx.Batch(batch)
		b.documentsMu.Unlock()
		if err != nil {
			b.endIndexing()
			return err
		}
	}

	// Always update languages, even if empty, to ensure consistency
	languagesStr := ""
	if len(languages) > 0 {
		languagesStr = strings.Join(languages, ",")
	}
	internalBatch := b.documentsIdx.NewBatch()
	internalBatch.SetInternal(internalLanguages, []byte(languagesStr))
	internalBatch.SetInternal(internalIllustratedMinSize, []byte(strconv.FormatFloat(b.illustratedMinSize, 'g', -1, 64)))
	internalBatch.SetInternal(internalMinOccurrenceRatio, []byte(strconv.FormatFloat(b.minOccurrenceRatio, 'g', -1, 64)))
	b.documentsMu.Lock()
	err = b.documentsIdx.Batch(internalBatch)
	b.documentsMu.Unlock()
	if err != nil {
		b.endIndexing()
		return err
	}

	if err := b.RebuildAuthorsFromDocuments(batchSize); err != nil {
		b.endIndexing()
		return err
	}

	b.endIndexing()
	return nil
}

func (b *BleveIndexer) collectPendingLibraryPaths(forceIndexing bool) (pending []string, languages []string, err error) {
	languages = []string{}
	e := afero.Walk(b.fs, b.libraryPath, func(fullPath string, f os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if f.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(fullPath))
		if _, ok := b.reader[ext]; !ok {
			return nil
		}
		if indexed, lang := b.isAlreadyIndexed(fullPath); indexed && !forceIndexing {
			b.indexedEntries.Add(1)
			languages = addLanguage(lang, languages)
			return nil
		}
		pending = append(pending, fullPath)
		return nil
	})
	return pending, languages, e
}

type metadataJobResult struct {
	path string
	meta metadata.Metadata
	err  error
}

// metadataJobResultFor extracts metadata (only) for a single path, shared by
// readMetadataForPaths' sequential and worker-pool branches. TextRank
// analysis, and for EPUBs the Words count, are deliberately not run here -
// see AddLibrary, EnrichTextRankKeywords and rankDocument.
func (b *BleveIndexer) metadataJobResultFor(path string) metadataJobResult {
	ext := strings.ToLower(filepath.Ext(path))
	meta, err := b.reader[ext].Metadata(path)
	return metadataJobResult{path: path, meta: meta, err: err}
}

func (b *BleveIndexer) readMetadataForPaths(paths []string, workers int) []metadataJobResult {
	out := make([]metadataJobResult, len(paths))
	parallelFor(len(paths), workers, func(i int) {
		out[i] = b.metadataJobResultFor(paths[i])
		b.indexedEntries.Add(1)
	})
	return out
}

// metadataAndKeywordsFor extracts fullPath's metadata and, if the registered
// reader for ext supports it, its TextRank keywords. When the reader also
// implements metadata.TextExtractor, its already-extracted text is reused
// for ranking instead of extracting and sanitizing the document a second
// time (see metadata.EpubReader.MetadataAndText).
func (b *BleveIndexer) metadataAndKeywordsFor(ext, fullPath string) (meta metadata.Metadata, phrases []string, words []string, err error) {
	reader := b.reader[ext]

	extractor, ok := reader.(metadata.TextExtractor)
	if !ok {
		meta, err = reader.Metadata(fullPath)
		return meta, nil, nil, err
	}

	meta, text, err := extractor.MetadataAndText(fullPath)
	if err != nil {
		return meta, nil, nil, err
	}
	phrases, words = b.rankTextFromContent(reader, text, fullPath)
	return meta, phrases, words, nil
}

// rankTextFromContent runs TextRank analysis on textContent (already
// extracted from fullPath) and returns its phrases and single words, one per
// element, ready to store in Document.TextRankPhrases/Document.TextRankWords
// for full-text search. Returns nil, nil (logging any error non-fatally) if
// reader doesn't implement metadata.TextRanker, ranking is disabled via
// b.minOccurrenceRatio, or the analysis fails.
func (b *BleveIndexer) rankTextFromContent(reader metadata.Reader, textContent, fullPath string) (phrases []string, words []string) {
	textRanker, ok := reader.(metadata.TextRanker)
	if !ok {
		return nil, nil
	}
	result, err := textRanker.RankText(b.minOccurrenceRatio, textContent, fullPath)
	if err != nil {
		log.Printf("Error ranking text for file %s: %s\n", fullPath, err)
		return nil, nil
	}
	if result == nil {
		return nil, nil
	}
	return textRankKeywords(result)
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

// documentsNeedingTextRank returns all documents not yet processed by
// EnrichTextRankKeywords (see Document.TextRankEnriched).
func (b *BleveIndexer) documentsNeedingTextRank() ([]Document, error) {
	notEnriched := bleve.NewBoolFieldQuery(false)
	notEnriched.SetField("TextRankEnriched")

	b.documentsMu.RLock()
	count, err := b.documentsIdx.DocCount()
	if err != nil {
		b.documentsMu.RUnlock()
		return nil, err
	}
	searchReq := bleve.NewSearchRequest(notEnriched)
	searchReq.Fields = []string{"*"}
	searchReq.Size = int(count)
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
// TextRankPhrases, TextRankWords, Words and TextRankEnriched set. Words is computed here
// rather than at AddLibrary time so that pass doesn't need to extract each
// EPUB's full text just to count words (see metadata.EpubReader.Metadata);
// it's instead counted from the same text this pass already extracts for
// TextRank. Safe to call concurrently across documents, since it only reads
// from b and returns a modified copy.
func (b *BleveIndexer) rankDocument(document Document) Document {
	fullPath := filepath.Join(b.libraryPath, document.ID)
	ext := strings.ToLower(filepath.Ext(fullPath))
	reader := b.reader[ext]

	if textSource, ok := reader.(metadata.TextSource); ok {
		if text, err := textSource.Text(fullPath); err != nil {
			log.Printf("Error extracting text for %s: %s\n", fullPath, err)
		} else {
			document.TextRankPhrases, document.TextRankWords = b.rankTextFromContent(reader, text, fullPath)
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
	pending, err := b.documentsNeedingTextRank()
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}

	log.Printf("Enriching %d documents with TextRank keywords", len(pending))
	b.beginTextRankEnrichment(len(pending))
	defer b.endTextRankEnrichment()

	for chunkStart := 0; chunkStart < len(pending); chunkStart += batchSize {
		chunk := pending[chunkStart:min(chunkStart+batchSize, len(pending))]
		enriched := b.rankDocuments(chunk, workers)

		batch := b.documentsIdx.NewBatch()
		for _, document := range enriched {
			if err := batch.Index(document.ID, document); err != nil {
				log.Printf("Error indexing enriched document %s: %s\n", document.ID, err)
			}
		}

		b.documentsMu.Lock()
		err := b.documentsIdx.Batch(batch)
		b.documentsMu.Unlock()
		if err != nil {
			return err
		}
	}

	log.Printf("TextRank enrichment finished")
	return nil
}

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
const maxIndexedTextRankEntries = 500

// textRankKeywords turns a TextRankResult's phrases and single words into two
// slices - one string per phrase, one per single word - ready to store in
// Document.TextRankPhrases and Document.TextRankWords respectively, one per
// slice element (rather than a single flattened string) so each lands in its
// own array entry - see the Document.TextRankPhrases doc comment for why that
// separation matters. Each slice is ordered by descending Weight (TextRank's
// own, per-document-normalized importance score), so a caller that only uses
// a prefix (e.g. subjectsQuery's Config.MaxSimilarityPhrases cap on
// TextRankPhrases) gets the most representative entries first rather than an
// arbitrary subset, and independently capped at maxIndexedTextRankEntries
// (see its own doc comment).
func textRankKeywords(result *metadata.TextRankResult) (phrases []string, words []string) {
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
// slice out of n entries, each produced by at(i). Shared by textRankKeywords
// for its Phrases/SingleWords slices, which come from different underlying
// types (rank.Phrase/rank.SingleWord) with no common interface.
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

func (b *BleveIndexer) isAlreadyIndexed(fullPath string) (bool, string) {
	b.documentsMu.RLock()
	doc, err := b.documentsIdx.Document(b.id(fullPath))
	b.documentsMu.RUnlock()
	if err != nil {
		log.Fatalln(err)
	}
	if doc == nil {
		return false, ""
	}
	lang := ""
	doc.VisitFields(func(f index.Field) {
		if f.Name() == "Language" {
			lang = string(f.Value())
			return
		}
	})
	return true, lang
}

func addLanguage(lang string, languages []string) []string {
	if !slices.Contains(languages, defaultAnalyzer) && lang == "" {
		return append(languages, defaultAnalyzer)
	}

	if _, ok := noStopWordsFilters[lang]; ok {
		if !slices.Contains(languages, lang) {
			languages = append(languages, lang)
		}
	}
	return languages
}

func (b *BleveIndexer) createDocument(meta metadata.Metadata, fullPath string, batchSlugs map[string]struct{}, documentsSeen map[string]Document) Document {
	document := Document{
		ID:                b.id(fullPath),
		Metadata:          meta,
		Slug:              slug.Make(meta.Title),
		AuthorsSlugs:      make([]string, len(meta.Authors)),
		IllustratorsSlugs: make([]string, len(meta.Illustrators)),
		SeriesSlug:        slug.Make(meta.Series),
		SubjectsSlugs:     make([]string, len(meta.Subjects)),
	}

	document.Slug = b.Slug(document, batchSlugs, documentsSeen)
	if documentsSeen != nil {
		documentsSeen[document.Slug] = document
	}

	for i, author := range meta.Authors {
		document.AuthorsSlugs[i] = slug.Make(author)
	}

	for i, illustrator := range meta.Illustrators {
		document.IllustratorsSlugs[i] = slug.Make(illustrator)
	}

	for i, subject := range meta.Subjects {
		document.SubjectsSlugs[i] = slug.Make(subject)
	}

	return document
}

// As Bleve index is not updated until the batch is executed, we need to store the slugs
// processed in the current batch in memory to also compare the current doc slug against them.
func (b *BleveIndexer) Slug(document Document, batchSlugs map[string]struct{}, documentsSeen map[string]Document) string {
	docSlug := makeDocumentSlug(document)
	i := 1
	existsInBatch := false
	for {
		doc, _ := b.documentBySlug(docSlug, documentsSeen)
		if batchSlugs != nil {
			_, existsInBatch = batchSlugs[docSlug]
		}
		if doc.Slug == docSlug && doc.ID == document.ID {
			return docSlug
		}
		if doc.Slug == "" && !existsInBatch {
			return docSlug
		}
		if documentSlugCollisionPattern.MatchString(docSlug) {
			pos := strings.LastIndex(docSlug, "--")
			docSlug = docSlug[:pos]
		}
		i++
		docSlug = fmt.Sprintf("%s--%d", docSlug, i)
	}
}

func (b *BleveIndexer) documentBySlug(docSlug string, documentsSeen map[string]Document) (Document, error) {
	if documentsSeen != nil {
		if doc, ok := documentsSeen[docSlug]; ok {
			return doc, nil
		}
	}
	doc, err := b.Document(docSlug)
	if err != nil {
		return Document{}, err
	}
	if documentsSeen != nil {
		documentsSeen[docSlug] = doc
	}
	return doc, nil
}

func (b *BleveIndexer) id(file string) string {
	ID := strings.ReplaceAll(file, b.libraryPath, "")
	return strings.TrimPrefix(ID, string(filepath.Separator))
}

func makeDocumentSlug(doc Document) string {
	docSlug := doc.Title
	if len(doc.Authors) > 0 {
		docSlug = strings.Join(append(doc.Authors, docSlug), "-")
	}

	return slug.MakeLang(docSlug, doc.Language)
}
