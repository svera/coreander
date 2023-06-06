package index

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/gosimple/slug"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v3/internal/metadata"
)

// AddFile adds a file to the index
func (b *BleveIndexer) AddFile(file string) error {
	ext := strings.ToLower(filepath.Ext(file))
	if _, ok := b.reader[ext]; !ok {
		return nil
	}
	meta, err := b.reader[ext].Metadata(file)
	if err != nil {
		return fmt.Errorf("error extracting metadata from file %s: %s", file, err)
	}

	docSlug := makeSlug(meta)
	meta = b.setID(meta, file)
	meta.Slug = b.checkSlug(meta.ID, docSlug, nil)

	err = b.idx.Index(meta.ID, meta)
	if err != nil {
		return fmt.Errorf("error indexing file %s: %s", file, err)
	}
	return nil
}

// RemoveFile removes a file from the index
func (b *BleveIndexer) RemoveFile(file string) error {
	file = strings.Replace(file, b.libraryPath, "", 1)
	file = strings.TrimPrefix(file, "/")
	if err := b.idx.Delete(file); err != nil {
		return err
	}
	return nil
}

// AddLibrary scans <libraryPath> for books and adds them to the index in batches of <bathSize>
func (b *BleveIndexer) AddLibrary(fs afero.Fs, batchSize int) error {
	batch := b.idx.NewBatch()
	batchSlugs := make(map[string]struct{}, batchSize)
	e := afero.Walk(fs, b.libraryPath, func(fullPath string, f os.FileInfo, err error) error {
		ext := strings.ToLower(filepath.Ext(fullPath))
		if _, ok := b.reader[ext]; !ok {
			return nil
		}
		meta, err := b.reader[ext].Metadata(fullPath)
		if err != nil {
			log.Printf("Error extracting metadata from file %s: %s\n", fullPath, err)
			return nil
		}

		docSlug := makeSlug(meta)
		meta = b.setID(meta, fullPath)
		meta.Slug = b.checkSlug(meta.ID, docSlug, batchSlugs)
		batchSlugs[docSlug] = struct{}{}

		err = batch.Index(meta.ID, meta)
		if err != nil {
			log.Printf("Error indexing file %s: %s\n", fullPath, err)
			return nil
		}

		if batch.Size() == batchSize {
			b.idx.Batch(batch)
			batch.Reset()
			batchSlugs = make(map[string]struct{}, batchSize)
		}
		return nil
	})

	b.idx.Batch(batch)
	return e
}

// As Bleve index is not updated until the batch is executed, we need to store the slugs
// processed in the current batch in memory to also compare the current doc slug against them.
func (b *BleveIndexer) checkSlug(ID, docSlug string, batchSlugs map[string]struct{}) string {
	i := 1
	existsInBatch := false
	for {
		doc, _ := b.Document(docSlug)
		if batchSlugs != nil {
			_, existsInBatch = batchSlugs[docSlug]
		}
		if doc.Slug == docSlug && doc.ID == ID {
			return docSlug
		}
		if doc.Slug == "" && !existsInBatch {
			return docSlug
		}
		i++
		docSlug = fmt.Sprintf("%s-%d", docSlug, i)
	}
}

func (b *BleveIndexer) setID(meta metadata.Metadata, file string) metadata.Metadata {
	meta.ID = strings.ReplaceAll(file, b.libraryPath, "")
	meta.ID = strings.TrimPrefix(meta.ID, "/")

	return meta
}

func makeSlug(meta metadata.Metadata) string {
	docSlug := meta.Title
	if len(meta.Authors) > 0 {
		docSlug = strings.Join(meta.Authors, ", ") + "-" + docSlug
	}

	return slug.Make(docSlug)
}
