package index

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/blevesearch/bleve/v2"
)

// OpenAuthorsIndex upgrades version 2 without discarding stored author metadata.
// The previous index is kept only until the migrated index opens successfully.
func OpenAuthorsIndex(path string, batchSize int) (bleve.Index, error) {
	authors, err := bleve.Open(path)
	if errors.Is(err, bleve.ErrorIndexPathDoesNotExist) {
		backup := path + ".v2-backup"
		if _, statErr := os.Stat(backup); statErr == nil {
			// Recover an interrupted swap before treating the index as missing.
			if err := os.Rename(backup, path); err != nil {
				return nil, fmt.Errorf("restoring authors index: %w", err)
			}
			authors, err = bleve.Open(path)
		} else if !os.IsNotExist(statErr) {
			return nil, fmt.Errorf("checking authors index backup: %w", statErr)
		}
	}
	if err != nil {
		return nil, err
	}
	version, err := authors.GetInternal(internalVersion)
	if err != nil {
		return nil, errors.Join(err, authors.Close())
	}
	if string(version) == "2" {
		upgraded, err := migrateAuthorsIndex(authors, path, batchSize)
		if err != nil {
			return nil, errors.Join(err, authors.Close())
		}
		authors = upgraded
	} else if string(version) != AuthorVersion {
		return authors, nil
	}
	if err := os.RemoveAll(path + ".v2-backup"); err != nil {
		return nil, errors.Join(fmt.Errorf("removing previous authors index: %w", err), authors.Close())
	}
	return authors, nil
}

func migrateAuthorsIndex(authors bleve.Index, path string, batchSize int) (bleve.Index, error) {
	if batchSize <= 0 {
		return nil, fmt.Errorf("authors migration batch size must be positive")
	}
	backup := path + ".v2-backup"
	if _, err := os.Stat(backup); err == nil {
		return nil, fmt.Errorf("authors migration backup already exists: %s", backup)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	stagingDir, err := os.MkdirTemp(filepath.Dir(path), ".authors-migration-")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := os.RemoveAll(stagingDir); err != nil {
			log.Printf("Error removing authors migration staging directory: %s", err)
		}
	}()
	stagingPath := filepath.Join(stagingDir, "index")
	staged, err := bleve.New(stagingPath, CreateAuthorsMapping())
	if err != nil {
		return nil, err
	}
	if err := copyAuthors(authors, staged, batchSize); err != nil {
		return nil, errors.Join(err, staged.Close())
	}
	if err := staged.Close(); err != nil {
		return nil, err
	}
	if err := authors.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(path, backup); err != nil {
		return nil, err
	}
	if err := os.Rename(stagingPath, path); err != nil {
		return nil, errors.Join(err, os.Rename(backup, path))
	}
	log.Printf("Migrated authors index to version %s", AuthorVersion)
	return bleve.Open(path)
}

func copyAuthors(source, destination bleve.Index, batchSize int) error {
	for from := 0; ; from += batchSize {
		req := bleve.NewSearchRequestOptions(bleve.NewMatchAllQuery(), batchSize, from, false)
		req.Fields = []string{"*"}
		req.SortBy([]string{"_id"})
		result, err := source.Search(req)
		if err != nil {
			return err
		}
		if len(result.Hits) == 0 {
			break
		}
		batch := destination.NewBatch()
		for _, hit := range result.Hits {
			if err := batch.Index(hit.ID, hydrateAuthor(hit)); err != nil {
				return err
			}
		}
		if err := destination.Batch(batch); err != nil {
			return err
		}
	}
	return destination.SetInternal(internalVersion, []byte(AuthorVersion))
}
