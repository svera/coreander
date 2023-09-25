package index

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/gosimple/slug"
	"github.com/spf13/afero"
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

	document := DocumentWrite{
		Document: Document{
			Metadata: meta,
		},
	}

	docSlug := makeSlug(document)
	document = b.setID(document, file)
	document.Slug = b.checkSlug(document.ID, docSlug, nil)
	document.SeriesEq = strings.ReplaceAll(slug.Make(document.Series), "-", "")
	document.AuthorsEq = make([]string, len(document.Authors))
	copy(document.AuthorsEq, meta.Authors)
	for i := range document.AuthorsEq {
		document.AuthorsEq[i] = strings.ReplaceAll(slug.Make(document.AuthorsEq[i]), "-", "")
	}
	document.SubjectsEq = make([]string, len(document.Subjects))
	copy(document.SubjectsEq, meta.Subjects)
	for i := range document.SubjectsEq {
		document.SubjectsEq[i] = strings.ReplaceAll(slug.Make(document.SubjectsEq[i]), "-", "")
	}

	err = b.idx.Index(document.ID, document)
	if err != nil {
		return fmt.Errorf("error indexing file %s: %s", file, err)
	}
	return nil
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

		document := DocumentWrite{
			Document: Document{
				Metadata: meta,
			},
		}

		docSlug := makeSlug(document)
		document = b.setID(document, fullPath)
		document.Slug = b.checkSlug(document.ID, docSlug, batchSlugs)
		document.SeriesEq = strings.ReplaceAll(slug.Make(document.Series), "-", "")
		document.AuthorsEq = make([]string, len(document.Authors))
		copy(document.AuthorsEq, meta.Authors)
		for i := range document.AuthorsEq {
			document.AuthorsEq[i] = strings.ReplaceAll(slug.Make(document.AuthorsEq[i]), "-", "")
		}
		document.SubjectsEq = make([]string, len(document.Subjects))
		copy(document.SubjectsEq, meta.Subjects)
		for i := range document.SubjectsEq {
			document.SubjectsEq[i] = strings.ReplaceAll(slug.Make(document.SubjectsEq[i]), "-", "")
		}

		batchSlugs[docSlug] = struct{}{}

		err = batch.Index(document.ID, document)
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

func (b *BleveIndexer) setID(meta DocumentWrite, file string) DocumentWrite {
	meta.ID = strings.ReplaceAll(file, b.libraryPath, "")
	meta.ID = strings.TrimPrefix(meta.ID, "/")

	return meta
}

func makeSlug(meta DocumentWrite) string {
	docSlug := meta.Title
	if len(meta.Authors) > 0 {
		docSlug = strings.Join(meta.Authors, ", ") + "-" + docSlug
	}

	return slug.Make(docSlug)
}
