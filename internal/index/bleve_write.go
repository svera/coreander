package index

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

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

	file = strings.Replace(file, b.libraryPath, "", 1)
	file = strings.TrimPrefix(file, "/")
	err = b.idx.Index(file, meta)
	if err != nil {
		return fmt.Errorf("error indexing file %s: %s", file, err)
	}
	return nil
}

// RemoveFile removes a file from the index
func (b *BleveIndexer) RemoveFile(file string) error {
	file = strings.Replace(file, b.libraryPath, "", 1)
	file = strings.TrimPrefix(file, "/")
	err := b.idx.Delete(file)
	if err != nil {
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

		err = batch.Index(path.Base(fullPath), meta)
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
