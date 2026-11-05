package index

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/blevesearch/bleve/v2"
)

func RecoverDocumentsIndex(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	backup := path + ".v13-backup"
	if _, err := os.Stat(backup); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.Rename(backup, path)
}

func MigrateDocumentsIndexV13(documents bleve.Index, path string, batchSize int) (bleve.Index, error) {
	if batchSize <= 0 {
		return nil, fmt.Errorf("documents migration batch size must be positive")
	}
	backup := path + ".v13-backup"
	if _, err := os.Stat(backup); err == nil {
		return nil, fmt.Errorf("documents migration backup already exists: %s", backup)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	stagingDir, err := os.MkdirTemp(filepath.Dir(path), ".documents-migration-")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := os.RemoveAll(stagingDir); err != nil {
			log.Printf("Error removing documents migration staging directory: %s", err)
		}
	}()

	stagingPath := filepath.Join(stagingDir, "index")
	staged, err := bleve.New(stagingPath, CreateDocumentsMapping())
	if err != nil {
		return nil, err
	}
	if err := copyDocumentsV13(documents, staged, batchSize); err != nil {
		return nil, errors.Join(err, staged.Close())
	}
	if err := staged.Close(); err != nil {
		return nil, err
	}
	if err := documents.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(path, backup); err != nil {
		return nil, err
	}
	if err := os.Rename(stagingPath, path); err != nil {
		return nil, errors.Join(err, os.Rename(backup, path))
	}
	migrated, err := bleve.Open(path)
	if err != nil {
		return nil, errors.Join(err, os.Rename(backup, path))
	}
	if err := os.RemoveAll(backup); err != nil {
		return nil, errors.Join(fmt.Errorf("removing previous documents index: %w", err), migrated.Close())
	}
	return migrated, nil
}

func copyDocumentsV13(source, destination bleve.Index, batchSize int) error {
	for from := 0; ; from += batchSize {
		request := bleve.NewSearchRequestOptions(bleve.NewMatchAllQuery(), batchSize, from, false)
		request.Fields = []string{"*"}
		request.SortBy([]string{"_id"})
		result, err := source.Search(request)
		if err != nil {
			return err
		}
		if len(result.Hits) == 0 {
			break
		}
		batch := destination.NewBatch()
		for _, hit := range result.Hits {
			document := hydrateDocument(hit)
			if err := IndexDocument(batch, hit.ID, document); err != nil {
				return err
			}
		}
		if err := destination.Batch(batch); err != nil {
			return err
		}
	}

	for _, key := range [][]byte{
		internalLanguages,
		internalIllustratedMinSize,
		internalMinOccurrenceRatio,
		internalMaxTextRankWords,
		internalDocCountAtLastPrune,
	} {
		value, err := source.GetInternal(key)
		if err != nil {
			return err
		}
		if value != nil {
			if err := destination.SetInternal(key, value); err != nil {
				return err
			}
		}
	}
	return destination.SetInternal(internalVersion, []byte(DocumentVersion))
}
