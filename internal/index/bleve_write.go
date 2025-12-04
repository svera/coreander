package index

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/blevesearch/bleve/v2"
	index "github.com/blevesearch/bleve_index_api"
	"github.com/gosimple/slug"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/metadata"
)

// AddFile adds a file to the index
func (b *BleveIndexer) AddFile(file string) (string, error) {
	ext := strings.ToLower(filepath.Ext(file))
	if _, ok := b.reader[ext]; !ok {
		return "", fmt.Errorf("file extension %s not supported", ext)
	}
	meta, err := b.reader[ext].Metadata(file)
	if err != nil {
		return "", fmt.Errorf("error extracting metadata from file %s: %s", file, err)
	}

	document := b.createDocument(meta, file, nil)
	document.AddedOn = time.Now().UTC()

	if err = b.documentsIdx.Index(document.ID, document); err != nil {
		return "", fmt.Errorf("error indexing file %s: %s", file, err)
	}

	// Index authors in the separate authors index
	if b.authorsIdx != nil {
		authorsBatch := b.authorsIdx.NewBatch()
		if err := b.indexAuthors(document, authorsBatch.Index); err != nil {
			return document.Slug, err
		}
		if authorsBatch.Size() > 0 {
			if err = b.authorsIdx.Batch(authorsBatch); err != nil {
				return document.Slug, err
			}
		}
	}

	return document.Slug, nil
}

// RemoveFile removes a file from the index
func (b *BleveIndexer) RemoveFile(file string) error {
	file = strings.Replace(file, b.libraryPath, "", 1)
	file = strings.TrimPrefix(file, string(filepath.Separator))

	// Get the document before deleting to update authors
	doc, err := b.DocumentByID(file)
	if err != nil {
		return err
	}

	// Delete the document
	if err := b.documentsIdx.Delete(file); err != nil {
		return err
	}

	// Update authors' subjects after document removal
	if doc.Slug != "" && b.authorsIdx != nil {
		authorsBatch := b.authorsIdx.NewBatch()
		for _, authorSlug := range doc.AuthorsSlugs {
			if authorSlug == "" {
				continue
			}

			existingAuthor, err := b.Author(authorSlug, "")
			if err != nil || existingAuthor.Slug == "" {
				continue
			}

			// Re-aggregate subjects from remaining documents
			subjects, subjectsSlugs := b.aggregateSubjectsForAuthor(authorSlug)
			existingAuthor.Subjects = subjects
			existingAuthor.SubjectsSlugs = subjectsSlugs

			if err := authorsBatch.Index(existingAuthor.Slug, existingAuthor); err != nil {
				log.Printf("Error updating author %s after document removal: %s\n", authorSlug, err)
				continue
			}
		}
		if authorsBatch.Size() > 0 {
			if err := b.authorsIdx.Batch(authorsBatch); err != nil {
				log.Printf("Error updating authors batch after document removal: %v\n", err)
			}
		}
	}

	return nil
}

// AddLibrary scans <libraryPath> for documents and adds them to the index in batches of <batchSize> if they
// haven't been previously indexed or if <forceIndexing> is true
func (b *BleveIndexer) AddLibrary(batchSize int, forceIndexing bool) error {
	batch := b.documentsIdx.NewBatch()
	var authorsBatch *bleve.Batch
	authorsBatchCreated := false
	batchSlugs := make(map[string]struct{}, batchSize)
	languages := []string{}
	b.indexStartTime = float64(time.Now().UnixNano())

	e := afero.Walk(b.fs, b.libraryPath, func(fullPath string, f os.FileInfo, walkErr error) error {
		if indexed, lang := b.isAlreadyIndexed(fullPath); indexed && !forceIndexing {
			b.indexedEntries += 1
			languages = addLanguage(lang, languages)
			return nil
		}
		ext := strings.ToLower(filepath.Ext(fullPath))
		if _, ok := b.reader[ext]; !ok {
			return nil
		}
		meta, err := b.reader[ext].Metadata(fullPath)
		if err != nil {
			log.Printf("Error extracting metadata from file %s: %s\n", fullPath, err)
			return nil
		}

		document := b.createDocument(meta, fullPath, batchSlugs)
		batchSlugs[document.Slug] = struct{}{}
		languages = addLanguage(meta.Language, languages)
		document.AddedOn = time.Time{}

		if err = batch.Index(document.ID, document); err != nil {
			log.Printf("Error indexing file %s: %s\n", fullPath, err)
			return nil
		}

		// Create authors batch lazily when we encounter the first document with authors
		if b.authorsIdx != nil && len(document.Authors) > 0 && !authorsBatchCreated {
			authorsBatch = b.authorsIdx.NewBatch()
			authorsBatchCreated = true
		}

		// Add authors to the authors batch
		if b.authorsIdx != nil && authorsBatch != nil {
			if err = b.indexAuthors(document, authorsBatch.Index); err != nil {
				return err
			}
		}

		b.indexedEntries += 1

		if batch.Size() >= batchSize {
			if err = b.documentsIdx.Batch(batch); err != nil {
				return err
			}
			batch.Reset()
			batchSlugs = make(map[string]struct{}, batchSize)
		}

		// Flush authors batch periodically
		if b.authorsIdx != nil && authorsBatch != nil && authorsBatch.Size() >= batchSize {
			if err = b.authorsIdx.Batch(authorsBatch); err != nil {
				return err
			}
			authorsBatch.Reset()
		}

		return nil
	})

	// Always update languages, even if empty, to ensure consistency
	languagesStr := ""
	if len(languages) > 0 {
		languagesStr = strings.Join(languages, ",")
	}
	batch.SetInternal(internalLanguages, []byte(languagesStr))

	// Flush remaining documents batch
	if err := b.documentsIdx.Batch(batch); err != nil {
		return err
	}

	// Flush remaining authors batch
	if b.authorsIdx != nil && authorsBatch != nil && authorsBatch.Size() > 0 {
		if err := b.authorsIdx.Batch(authorsBatch); err != nil {
			return err
		}
	}

	// Update all authors' subjects now that all documents are committed
	if err := b.updateAllAuthorsSubjects(); err != nil {
		log.Printf("Warning: Could not update authors' subjects: %v\n", err)
	}

	b.indexStartTime = 0
	b.indexedEntries = 0
	return e
}

// updateAllAuthorsSubjects updates subjects for all authors based on their documents
func (b *BleveIndexer) updateAllAuthorsSubjects() error {
	if b.authorsIdx == nil || b.documentsIdx == nil {
		return nil
	}

	// Get all authors
	matchAllQuery := bleve.NewMatchAllQuery()
	searchRequest := bleve.NewSearchRequest(matchAllQuery)
	searchRequest.Size = 10000 // Adjust if needed
	searchRequest.Fields = []string{"Slug"}

	authorsResult, err := b.authorsIdx.Search(searchRequest)
	if err != nil {
		return err
	}
	if authorsResult == nil || authorsResult.Total == 0 {
		return nil
	}

	// Update each author's subjects
	authorsBatch := b.authorsIdx.NewBatch()
	for _, hit := range authorsResult.Hits {
		authorSlug := ""
		if slugVal, ok := hit.Fields["Slug"]; ok {
			if slugStr, ok := slugVal.(string); ok {
				authorSlug = slugStr
			}
		}
		if authorSlug == "" {
			authorSlug = hit.ID
		}

		// Get the full author
		author, err := b.Author(authorSlug, "")
		if err != nil || author.Slug == "" {
			continue
		}

		// Aggregate subjects
		subjects, subjectsSlugs := b.aggregateSubjectsForAuthor(authorSlug)
		author.Subjects = subjects
		author.SubjectsSlugs = subjectsSlugs

		if err := authorsBatch.Index(author.Slug, author); err != nil {
			log.Printf("Error updating author %s subjects: %v\n", authorSlug, err)
			continue
		}
	}

	if authorsBatch.Size() > 0 {
		return b.authorsIdx.Batch(authorsBatch)
	}

	return nil
}

// aggregateSubjectsForAuthor collects all unique subjects and subject slugs from all documents by an author
func (b *BleveIndexer) aggregateSubjectsForAuthor(authorSlug string) ([]string, []string) {
	if b.documentsIdx == nil {
		return []string{}, []string{}
	}

	query := bleve.NewTermQuery(authorSlug)
	query.SetField("AuthorsSlugs")

	searchRequest := bleve.NewSearchRequest(query)
	searchRequest.Size = 10000 // Get all documents (adjust if needed for very large libraries)
	searchRequest.Fields = []string{"Subjects", "SubjectsSlugs"}

	searchResult, err := b.documentsIdx.Search(searchRequest)
	if err != nil {
		// Silently return empty subjects if query fails
		return []string{}, []string{}
	}

	subjectsMap := make(map[string]struct{})
	subjectsSlugsMap := make(map[string]struct{})

	for _, hit := range searchResult.Hits {
		if subjects, ok := hit.Fields["Subjects"]; ok && subjects != nil {
			if subjectsSlice, ok := subjects.([]any); ok {
				for _, subject := range subjectsSlice {
					if subjectStr, ok := subject.(string); ok && subjectStr != "" {
						subjectsMap[subjectStr] = struct{}{}
					}
				}
			} else if subjectStr, ok := subjects.(string); ok && subjectStr != "" {
				// Handle case where Bleve returns a single string instead of slice
				subjectsMap[subjectStr] = struct{}{}
			}
		}

		if subjectsSlugs, ok := hit.Fields["SubjectsSlugs"]; ok && subjectsSlugs != nil {
			if slugsSlice, ok := subjectsSlugs.([]any); ok {
				for _, slug := range slugsSlice {
					if slugStr, ok := slug.(string); ok && slugStr != "" {
						subjectsSlugsMap[slugStr] = struct{}{}
					}
				}
			} else if slugStr, ok := subjectsSlugs.(string); ok && slugStr != "" {
				// Handle case where Bleve returns a single string instead of slice
				subjectsSlugsMap[slugStr] = struct{}{}
			}
		}
	}

	subjects := make([]string, 0, len(subjectsMap))
	for subject := range subjectsMap {
		subjects = append(subjects, subject)
	}
	slices.Sort(subjects)

	subjectsSlugs := make([]string, 0, len(subjectsSlugsMap))
	for slug := range subjectsSlugsMap {
		subjectsSlugs = append(subjectsSlugs, slug)
	}
	slices.Sort(subjectsSlugs)

	return subjects, subjectsSlugs
}

// indexAuthors indexes authors of a document if they are not already indexed in the authors index
// During batch operations, it creates/updates authors without checking for existing ones to avoid
// issues with uncommitted batches. Subjects are aggregated from committed documents only.
func (b *BleveIndexer) indexAuthors(document Document, index func(id string, data any) error) error {
	for i, name := range document.Authors {
		// Skip authors with empty names or empty slugs
		if name == "" || document.AuthorsSlugs[i] == "" {
			continue
		}

		authorSlug := document.AuthorsSlugs[i]

		// Create/update author - during batch operations we don't check for existing authors
		// to avoid issues with uncommitted batches. The index function will handle updates.
		// Subjects will be populated after batch commit via updateAllAuthorsSubjects()
		author := Author{
			Name:          name,
			Slug:          authorSlug,
			RetrievedOn:   time.Time{},
			WikipediaLink: make(map[string]string),
			Description:   make(map[string]string),
			Pseudonyms:    []string{},
			Subjects:      []string{},
			SubjectsSlugs: []string{},
		}

		if err := index(author.Slug, author); err != nil {
			log.Printf("Error indexing author %s: %s\n", name, err)
			continue
		}
	}
	return nil
}

func (b *BleveIndexer) IndexAuthor(author Author) error {
	if b.authorsIdx == nil {
		return fmt.Errorf("authors index is not initialized")
	}
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

func (b *BleveIndexer) createDocument(meta metadata.Metadata, fullPath string, batchSlugs map[string]struct{}) Document {
	document := Document{
		ID:            b.id(fullPath),
		Metadata:      meta,
		Slug:          slug.Make(meta.Title),
		AuthorsSlugs:  make([]string, len(meta.Authors)),
		SeriesSlug:    slug.Make(meta.Series),
		SubjectsSlugs: make([]string, len(meta.Subjects)),
	}

	document.Slug = b.Slug(document, batchSlugs)

	for i, author := range meta.Authors {
		document.AuthorsSlugs[i] = slug.Make(author)
	}

	for i, subject := range meta.Subjects {
		document.SubjectsSlugs[i] = slug.Make(subject)
	}

	return document
}

// As Bleve index is not updated until the batch is executed, we need to store the slugs
// processed in the current batch in memory to also compare the current doc slug against them.
func (b *BleveIndexer) Slug(document Document, batchSlugs map[string]struct{}) string {
	docSlug := makeDocumentSlug(document)
	exp, err := regexp.Compile(`^[a-zA-Z0-9\-]+(--)[0-9]+$`)
	if err != nil {
		log.Fatal(err)
	}
	i := 1
	existsInBatch := false
	for {
		doc, _ := b.Document(docSlug)
		if batchSlugs != nil {
			_, existsInBatch = batchSlugs[docSlug]
		}
		if doc.Slug == docSlug && doc.ID == document.ID {
			return docSlug
		}
		if doc.Slug == "" && !existsInBatch {
			return docSlug
		}
		if exp.MatchString(docSlug) {
			pos := strings.LastIndex(docSlug, "--")
			docSlug = docSlug[:pos]
		}
		i++
		docSlug = fmt.Sprintf("%s--%d", docSlug, i)
	}
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
