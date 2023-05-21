package index

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/gosimple/slug"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v2/internal/metadata"
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

	meta = b.setFilenameAndID(meta, file)

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

		meta = b.setFilenameAndID(meta, fullPath)

		err = batch.Index(meta.ID, meta)
		if err != nil {
			log.Printf("Error indexing file %s: %s\n", fullPath, err)
			return nil
		}

		if batch.Size() == batchSize {
			b.idx.Batch(batch)
			batch.Reset()
		}
		return nil
	})

	b.idx.Batch(batch)
	return e
}

func (b *BleveIndexer) setFilenameAndID(meta metadata.Metadata, file string) metadata.Metadata {
	slugSource := meta.Title
	if len(meta.Authors) > 0 {
		slugSource = strings.Join(meta.Authors, ", ") + "-" + slugSource
	}

	docSlug := slug.Make(slugSource)

	i := 1
	for {
		if doc, _ := b.idx.Document(docSlug); doc == nil {
			break
		}
		i++
		docSlug = fmt.Sprintf("%s_%d", docSlug, i)
	}

	meta.ID = docSlug
	meta.Filename = strings.ReplaceAll(file, b.libraryPath, "")
	meta.Filename = strings.TrimPrefix(meta.Filename, "/")

	return meta
}
