package index

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/gosimple/slug"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/metadata"
)

// documentSlugCollisionPattern matches slugs like "title--2" used for disambiguation.
var documentSlugCollisionPattern = regexp.MustCompile(`^[a-zA-Z0-9\-]+(--)[0-9]+$`)

func (b *BleveIndexer) IndexingProgress() (Progress, error) {
	if b.indexProgress.startNanos.Load() != 0 {
		return b.progressFrom(ProgressDocuments, b.indexProgress.startNanos.Load(), b.indexProgress.processed.Load(), b.indexProgress.total.Load()), nil
	}
	if b.authorEnrichProgress.startNanos.Load() != 0 {
		return b.progressFrom(ProgressAuthors, b.authorEnrichProgress.startNanos.Load(), b.authorEnrichProgress.processed.Load(), b.authorEnrichProgress.total.Load()), nil
	}
	if b.textRankEnrichProgress.startNanos.Load() != 0 {
		return b.progressFrom(ProgressTextRank, b.textRankEnrichProgress.startNanos.Load(), b.textRankEnrichProgress.processed.Load(), b.textRankEnrichProgress.total.Load()), nil
	}
	if b.pruneProgress.startNanos.Load() != 0 {
		return b.progressFrom(ProgressPruning, b.pruneProgress.startNanos.Load(), b.pruneProgress.processed.Load(), b.pruneProgress.total.Load()), nil
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
	b.indexProgress.begin(0)
}

func (b *BleveIndexer) endIndexing() {
	b.indexProgress.end()
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

// indexFile adds a file to the index. Like AddLibrary, this is a fast,
// metadata-only pass: TextRank analysis is deferred to a background
// goroutine (see enrichTextRankAndReindex) instead of running inline, so
// callers - NewFile (document upload) and the file watcher - don't block on
// it for potentially large documents.
func (b *BleveIndexer) indexFile(file string) (string, error) {
	ext := strings.ToLower(filepath.Ext(file))
	if _, ok := b.reader[ext]; !ok {
		return "", fmt.Errorf("file extension %s not supported", ext)
	}

	id := b.id(file)
	unlock := b.lockFile(id)
	defer unlock()

	meta, err := b.reader[ext].Metadata(file)
	if err != nil {
		return "", fmt.Errorf("error extracting metadata from file %s: %s", file, err)
	}

	// Uploading a file (NewFile) writes it to disk and indexes it directly,
	// but that same write also triggers the file watcher's own indexFile
	// call for the same path. fileLocks serializes the two calls; whichever
	// runs second lands here and finds, via lastIndexed, the document the
	// other one already indexed. If its metadata is unchanged, this is that
	// duplicate event, not a real content change, so skip re-indexing rather
	// than picking a second, possibly colliding slug and double-counting
	// author stats.
	if existingIface, ok := b.lastIndexed.Load(id); ok {
		existing := existingIface.(Document)
		if reflect.DeepEqual(existing.Metadata, meta) {
			return existing.Slug, nil
		}
	}

	document := b.createDocument(meta, file, nil, nil)
	document.AddedOn = time.Now().UTC()
	document.TextRankEnriched = !b.supportsTextRank(file)

	b.documentsMu.Lock()
	err = b.documentsIdx.Index(document.ID, document)
	b.documentsMu.Unlock()
	if err != nil {
		return "", fmt.Errorf("error indexing file %s: %s", file, err)
	}
	b.lastIndexed.Store(id, document)

	if err := b.incrementAuthorCounts(document.Authors, document.AuthorsSlugs); err != nil {
		return document.Slug, err
	}
	if err := b.incrementAuthorCounts(document.Illustrators, document.IllustratorsSlugs); err != nil {
		return document.Slug, err
	}

	if !document.TextRankEnriched {
		b.scheduleTextRankEnrichment(document)
	}

	b.maybePruneForLibraryChange()

	return document.Slug, nil
}

// lockFile returns an unlock function for a mutex scoped to the given
// document ID, creating it on first use. See fileLocks.
func (b *BleveIndexer) lockFile(id string) func() {
	muIface, _ := b.fileLocks.LoadOrStore(id, &sync.Mutex{})
	mu := muIface.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
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
	err = b.documentsIdx.Delete(id)
	b.documentsMu.Unlock()
	if err != nil {
		return err
	}
	b.maybePruneForLibraryChange()
	return nil
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
	b.maybePruneForLibraryChange()
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
	b.indexProgress.total.Store(b.indexProgress.processed.Load() + uint64(len(pending)))
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

	indexedLanguages, err := b.indexedDocumentLanguages()
	if err != nil {
		return nil, nil, err
	}

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
		if lang, indexed := indexedLanguages[b.id(fullPath)]; indexed && !forceIndexing {
			b.indexProgress.processed.Add(1)
			languages = addLanguage(lang, languages)
			return nil
		}
		pending = append(pending, fullPath)
		return nil
	})
	return pending, languages, e
}

// indexedDocumentLanguages returns every already-indexed document's ID and
// Language in one fetch, requesting only that field so
// collectPendingLibraryPaths can check "already indexed" via a map lookup
// instead of one full-document (incl. TextRankPhrases/Words) read per file.
func (b *BleveIndexer) indexedDocumentLanguages() (map[string]string, error) {
	b.documentsMu.RLock()
	docCount, err := b.documentsIdx.DocCount()
	if err != nil {
		b.documentsMu.RUnlock()
		return nil, err
	}
	if docCount == 0 {
		b.documentsMu.RUnlock()
		return map[string]string{}, nil
	}

	searchReq := bleve.NewSearchRequestOptions(bleve.NewMatchAllQuery(), int(docCount), 0, false)
	searchReq.Fields = []string{"Language"}
	searchResult, err := b.documentsIdx.Search(searchReq)
	b.documentsMu.RUnlock()
	if err != nil {
		return nil, err
	}

	languages := make(map[string]string, len(searchResult.Hits))
	for _, hit := range searchResult.Hits {
		lang, _ := hit.Fields["Language"].(string)
		languages[hit.ID] = lang
	}
	return languages, nil
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
		b.indexProgress.processed.Add(1)
	})
	return out
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
