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

	err = b.idx.Index(document.ID, document)
	if err != nil {
		return "", fmt.Errorf("error indexing file %s: %s", file, err)
	}
	return document.Slug, nil
}

// RemoveFile removes a file from the index
func (b *BleveIndexer) RemoveFile(file string) error {
	file = strings.Replace(file, b.libraryPath, "", 1)
	file = strings.TrimPrefix(file, string(filepath.Separator))
	if err := b.idx.Delete(file); err != nil {
		return err
	}
	return nil
}

// AddLibrary scans <libraryPath> for documents and adds them to the index in batches of <batchSize> if they
// haven't been previously indexed or if <forceIndexing> is true
func (b *BleveIndexer) AddLibrary(batchSize int, forceIndexing bool) error {
	batch := b.idx.NewBatch()
	batchSlugs := make(map[string]struct{}, batchSize)
	languages := []string{}
	b.indexStartTime = float64(time.Now().UnixNano())
	e := afero.Walk(b.fs, b.libraryPath, func(fullPath string, f os.FileInfo, err error) error {
		if indexed, lang := b.isAlreadyIndexed(fullPath); indexed && !forceIndexing {
			b.indexedDocuments += 1
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

		err = batch.Index(document.ID, document)
		if err != nil {
			log.Printf("Error indexing file %s: %s\n", fullPath, err)
			return nil
		}

		b.indexedDocuments += 1
		if batch.Size() == batchSize {
			b.idx.Batch(batch)
			batch.Reset()
			batchSlugs = make(map[string]struct{}, batchSize)
		}
		return nil
	})
	if len(languages) > 0 {
		batch.SetInternal(internalLanguages, []byte(strings.Join(languages, ",")))
	}
	b.idx.Batch(batch)
	b.indexStartTime = 0
	b.indexedDocuments = 0
	return e
}

func (b *BleveIndexer) isAlreadyIndexed(fullPath string) (bool, string) {
	doc, err := b.idx.Document(b.id(fullPath))
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

func (b *BleveIndexer) createDocument(meta metadata.Metadata, fullPath string, batchSlugs map[string]struct{}) DocumentWrite {
	document := DocumentWrite{
		Document: Document{
			Metadata: meta,
		},
		SeriesEq:   strings.ReplaceAll(slug.Make(meta.Series), "-", ""),
		AuthorsEq:  make([]string, len(meta.Authors)),
		SubjectsEq: make([]string, len(meta.Subjects)),
	}

	document.ID = b.id(fullPath)
	document.Slug = b.Slug(document, batchSlugs)
	copy(document.AuthorsEq, meta.Authors)
	for i := range document.AuthorsEq {
		document.AuthorsEq[i] = strings.ReplaceAll(slug.Make(document.AuthorsEq[i]), "-", "")
	}
	copy(document.SubjectsEq, meta.Subjects)
	for i := range document.SubjectsEq {
		document.SubjectsEq[i] = strings.ReplaceAll(slug.Make(document.SubjectsEq[i]), "-", "")
	}

	return document
}

// As Bleve index is not updated until the batch is executed, we need to store the slugs
// processed in the current batch in memory to also compare the current doc slug against them.
func (b *BleveIndexer) Slug(document DocumentWrite, batchSlugs map[string]struct{}) string {
	docSlug := makeSlug(document)
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

func makeSlug(meta DocumentWrite) string {
	docSlug := meta.Title
	if len(meta.Authors) > 0 {
		docSlug = strings.Join(meta.Authors, ", ") + "-" + docSlug
	}

	return slug.MakeLang(docSlug, meta.Language)
}
