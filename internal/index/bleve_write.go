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
	authorsBatch := b.authorsIdx.NewBatch()
	if err := b.indexAuthors(document, authorsBatch.Index); err != nil {
		return document.Slug, err
	}
	if authorsBatch.Size() > 0 {
		if err = b.authorsIdx.Batch(authorsBatch); err != nil {
			return document.Slug, err
		}
	}

	return document.Slug, nil
}

// RemoveFile removes a file from the index
func (b *BleveIndexer) RemoveFile(file string) error {
	file = strings.Replace(file, b.libraryPath, "", 1)
	file = strings.TrimPrefix(file, string(filepath.Separator))
	if err := b.documentsIdx.Delete(file); err != nil {
		return err
	}
	return nil
}

// AddLibrary scans <libraryPath> for documents and adds them to the index in batches of <batchSize> if they
// haven't been previously indexed or if <forceIndexing> is true
func (b *BleveIndexer) AddLibrary(batchSize int, forceIndexing bool) error {
	batch := b.documentsIdx.NewBatch()
	authorsBatch := b.authorsIdx.NewBatch()
	batchSlugs := make(map[string]struct{}, batchSize)
	languages := []string{}
	b.indexStartTime = float64(time.Now().UnixNano())

	e := afero.Walk(b.fs, b.libraryPath, func(fullPath string, f os.FileInfo, err error) error {
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

		// Add authors to the authors batch
		if err = b.indexAuthors(document, authorsBatch.Index); err != nil {
			return err
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
		if authorsBatch.Size() >= batchSize {
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
	if authorsBatch.Size() > 0 {
		if err := b.authorsIdx.Batch(authorsBatch); err != nil {
			return err
		}
	}

	b.indexStartTime = 0
	b.indexedEntries = 0
	return e
}

// indexAuthors indexes authors of a document if they are not already indexed in the authors index
func (b *BleveIndexer) indexAuthors(document Document, index func(id string, data any) error) error {
	for i, name := range document.Authors {
		// Skip authors with empty names or empty slugs
		if name == "" || document.AuthorsSlugs[i] == "" {
			continue
		}

		// Check if author already exists in authors index
		indexedAuthor, err := b.authorsIdx.Document(document.AuthorsSlugs[i])
		if err != nil {
			return err
		}
		if indexedAuthor != nil {
			continue
		}

		author := Author{
			Name:        name,
			Slug:        document.AuthorsSlugs[i],
			RetrievedOn: time.Time{},
		}

		if err := index(author.Slug, author); err != nil {
			log.Printf("Error indexing author %s: %s\n", name, err)
			continue
		}
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
