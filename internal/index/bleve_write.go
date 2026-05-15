package index

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	index "github.com/blevesearch/bleve_index_api"
	"github.com/gosimple/slug"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/metadata"
)

// documentSlugCollisionPattern matches slugs like "title--2" used for disambiguation.
var documentSlugCollisionPattern = regexp.MustCompile(`^[a-zA-Z0-9\-]+(--)[0-9]+$`)

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
	meta, err := b.reader[ext].Metadata(file)
	if err != nil {
		return "", fmt.Errorf("error extracting metadata from file %s: %s", file, err)
	}

	document := b.createDocument(meta, file, nil, nil)
	document.AddedOn = time.Now().UTC()

	if err = b.documentsIdx.Index(document.ID, document); err != nil {
		return "", fmt.Errorf("error indexing file %s: %s", file, err)
	}

	// Index authors in the separate authors index
	authorsBatch := b.authorsIdx.NewBatch()
	if err := b.indexAuthors(document, authorsBatch.Index, nil); err != nil {
		return document.Slug, err
	}
	if authorsBatch.Size() > 0 {
		if err = b.authorsIdx.Batch(authorsBatch); err != nil {
			return document.Slug, err
		}
	}

	return document.Slug, nil
}

// removeFile removes a file from the index
func (b *BleveIndexer) removeFile(file string) error {
	file = strings.Replace(file, b.libraryPath, "", 1)
	file = strings.TrimPrefix(file, string(filepath.Separator))
	if err := b.documentsIdx.Delete(file); err != nil {
		return err
	}
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
	fullPath := filepath.Join(b.libraryPath, document.ID)
	if err := b.removeFile(fullPath); err != nil {
		return err
	}
	if err := b.fs.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		log.Printf("error removing file %s: %s\n", fullPath, err.Error())
	}
	return nil
}

// AddLibrary scans <libraryPath> for documents and adds them to the index in batches of <batchSize> if they
// haven't been previously indexed or if <forceIndexing> is true.
// metadataWorkers controls parallel metadata extraction: 0 or 1 is fully sequential; values greater than 1
// use a bounded worker pool while Bleve batching and slug resolution stay on a single goroutine.
func (b *BleveIndexer) AddLibrary(batchSize int, forceIndexing bool, metadataWorkers int) error {
	b.beginIndexing()

	pending, languages, err := b.collectPendingLibraryPaths(forceIndexing)
	if err != nil {
		b.endIndexing()
		return err
	}
	b.indexTotalEntries.Store(b.indexedEntries.Load() + uint64(len(pending)))
	slices.Sort(pending)

	metaJobs := b.readMetadataForPaths(pending, metadataWorkers)

	batch := b.documentsIdx.NewBatch()
	authorsBatch := b.authorsIdx.NewBatch()
	batchSlugs := make(map[string]struct{}, batchSize)
	slugLookaside := make(map[string]Document, batchSize*4)
	authorsSeen := make(map[string]struct{}, batchSize*4)

	for _, job := range metaJobs {
		if job.err != nil {
			log.Printf("Error extracting metadata from file %s: %s\n", job.path, job.err)
			continue
		}
		fullPath := job.path
		meta := job.meta

		document := b.createDocument(meta, fullPath, batchSlugs, slugLookaside)
		batchSlugs[document.Slug] = struct{}{}
		languages = addLanguage(meta.Language, languages)
		document.AddedOn = time.Time{}

		if err = batch.Index(document.ID, document); err != nil {
			log.Printf("Error indexing file %s: %s\n", fullPath, err)
			continue
		}

		if err = b.indexAuthors(document, authorsBatch.Index, authorsSeen); err != nil {
			b.endIndexing()
			return err
		}

		if batch.Size() >= batchSize {
			if err = b.documentsIdx.Batch(batch); err != nil {
				b.endIndexing()
				return err
			}
			batch.Reset()
			batchSlugs = make(map[string]struct{}, batchSize)
		}

		if authorsBatch.Size() >= batchSize {
			if err = b.authorsIdx.Batch(authorsBatch); err != nil {
				b.endIndexing()
				return err
			}
			authorsBatch.Reset()
		}
	}

	// Always update languages, even if empty, to ensure consistency
	languagesStr := ""
	if len(languages) > 0 {
		languagesStr = strings.Join(languages, ",")
	}
	batch.SetInternal(internalLanguages, []byte(languagesStr))
	batch.SetInternal(internalIllustratedMinSize, []byte(strconv.FormatFloat(b.illustratedMinSize, 'g', -1, 64)))

	// Flush remaining documents batch
	if err := b.documentsIdx.Batch(batch); err != nil {
		b.endIndexing()
		return err
	}

	// Flush remaining authors batch
	if authorsBatch.Size() > 0 {
		if err := b.authorsIdx.Batch(authorsBatch); err != nil {
			b.endIndexing()
			return err
		}
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

func (b *BleveIndexer) readMetadataForPaths(paths []string, workers int) []metadataJobResult {
	out := make([]metadataJobResult, len(paths))
	if len(paths) == 0 {
		return out
	}
	recordProgress := func() {
		b.indexedEntries.Add(1)
	}
	if workers <= 1 {
		for i, p := range paths {
			ext := strings.ToLower(filepath.Ext(p))
			meta, err := b.reader[ext].Metadata(p)
			out[i] = metadataJobResult{path: p, meta: meta, err: err}
			recordProgress()
		}
		return out
	}
	if workers > maxMetadataWorkers {
		workers = maxMetadataWorkers
	}
	if workers > len(paths) {
		workers = len(paths)
	}
	type indexedPath struct {
		i    int
		path string
	}
	jobs := make(chan indexedPath)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				ext := strings.ToLower(filepath.Ext(j.path))
				meta, err := b.reader[ext].Metadata(j.path)
				out[j.i] = metadataJobResult{path: j.path, meta: meta, err: err}
				recordProgress()
			}
		}()
	}
	for i, p := range paths {
		jobs <- indexedPath{i, p}
	}
	close(jobs)
	wg.Wait()
	return out
}

// indexAuthors indexes document authors and illustrators in the authors index when missing.
// authorsSeen, when non-nil, records author slugs already known or batched in this AddLibrary run
// to avoid repeated authorsIdx.Document lookups.
func (b *BleveIndexer) indexAuthors(document Document, index func(id string, data any) error, authorsSeen map[string]struct{}) error {
	for i, name := range document.Authors {
		if err := b.indexAuthorIfMissing(name, document.AuthorsSlugs[i], index, authorsSeen); err != nil {
			return err
		}
	}
	for _, name := range document.Illustrators {
		if err := b.indexAuthorIfMissing(name, slug.Make(name), index, authorsSeen); err != nil {
			return err
		}
	}
	return nil
}

func (b *BleveIndexer) indexAuthorIfMissing(name, authorSlug string, index func(id string, data any) error, authorsSeen map[string]struct{}) error {
	if name == "" || authorSlug == "" {
		return nil
	}
	if authorsSeen != nil {
		if _, ok := authorsSeen[authorSlug]; ok {
			return nil
		}
	}
	indexedAuthor, err := b.authorsIdx.Document(authorSlug)
	if err != nil {
		return err
	}
	if indexedAuthor != nil {
		if authorsSeen != nil {
			authorsSeen[authorSlug] = struct{}{}
		}
		return nil
	}
	author := Author{
		Name:        name,
		Slug:        authorSlug,
		RetrievedOn: time.Time{},
	}
	if err := index(author.Slug, author); err != nil {
		log.Printf("Error indexing author %s: %s\n", name, err)
	}
	if authorsSeen != nil {
		authorsSeen[authorSlug] = struct{}{}
	}
	return nil
}

func (b *BleveIndexer) IndexAuthor(author Author) error {
	if err := b.authorsIdx.Index(author.Slug, author); err != nil {
		return err
	}
	return nil
}

func (b *BleveIndexer) isAlreadyIndexed(fullPath string) (bool, string) {
	doc, err := b.documentsIdx.Document(b.id(fullPath))
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
		found := false
		for i := range languages {
			if languages[i] == lang {
				found = true
				break
			}
		}
		if !found {
			languages = append(languages, lang)
		}
	}
	return languages
}

func (b *BleveIndexer) createDocument(meta metadata.Metadata, fullPath string, batchSlugs map[string]struct{}, slugLookaside map[string]Document) Document {
	document := Document{
		ID:                b.id(fullPath),
		Metadata:          meta,
		Slug:              slug.Make(meta.Title),
		AuthorsSlugs:      make([]string, len(meta.Authors)),
		IllustratorsSlugs: make([]string, len(meta.Illustrators)),
		SeriesSlug:        slug.Make(meta.Series),
		SubjectsSlugs:     make([]string, len(meta.Subjects)),
	}

	document.Slug = b.Slug(document, batchSlugs, slugLookaside)

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
func (b *BleveIndexer) Slug(document Document, batchSlugs map[string]struct{}, slugLookaside map[string]Document) string {
	docSlug := makeDocumentSlug(document)
	i := 1
	existsInBatch := false
	for {
		doc, _ := b.documentBySlug(docSlug, slugLookaside)
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

func (b *BleveIndexer) documentBySlug(docSlug string, slugLookaside map[string]Document) (Document, error) {
	if slugLookaside != nil {
		if doc, ok := slugLookaside[docSlug]; ok {
			return doc, nil
		}
	}
	doc, err := b.Document(docSlug)
	if err != nil {
		return Document{}, err
	}
	if slugLookaside != nil {
		slugLookaside[docSlug] = doc
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
