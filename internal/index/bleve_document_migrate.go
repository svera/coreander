package index

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/blevesearch/bleve/v2"
	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
)

var previousDocumentVersions = []string{"v13", "v14", "v15", "v16"}

// OpenDocumentsIndex preserves document metadata while upgrading provider storage.
func OpenDocumentsIndex(path string, batchSize int) (bleve.Index, error) {
	documents, err := bleve.Open(path)
	for _, version := range previousDocumentVersions {
		if !errors.Is(err, bleve.ErrorIndexPathDoesNotExist) {
			break
		}
		backup := path + "." + version + "-backup"
		if _, statErr := os.Stat(backup); statErr == nil {
			if err := os.Rename(backup, path); err != nil {
				return nil, fmt.Errorf("restoring documents index: %w", err)
			}
			documents, err = bleve.Open(path)
		} else if !os.IsNotExist(statErr) {
			return nil, fmt.Errorf("checking documents index backup: %w", statErr)
		}
	}
	if err != nil {
		return nil, err
	}
	version, err := documents.GetInternal(internalVersion)
	if err != nil {
		return nil, errors.Join(err, documents.Close())
	}
	for _, previous := range previousDocumentVersions {
		if string(version) == previous {
			upgraded, err := migrateStoredIndex(documents, path, path+"."+previous+"-backup", batchSize, CreateDocumentsMapping(), copyDocuments)
			if err != nil {
				return nil, errors.Join(err, documents.Close())
			}
			documents = upgraded
			version = []byte(DocumentVersion)
			log.Printf("Migrated documents index to version %s", DocumentVersion)
			break
		}
	}
	if string(version) != DocumentVersion {
		return nil, errors.Join(fmt.Errorf("unsupported documents index version %q; index was not modified", version), documents.Close())
	}
	for _, previous := range previousDocumentVersions {
		if err := os.RemoveAll(path + "." + previous + "-backup"); err != nil {
			return nil, errors.Join(fmt.Errorf("removing previous documents index: %w", err), documents.Close())
		}
	}
	return documents, nil
}

func copyDocuments(source, destination bleve.Index, batchSize int) error {
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
			document := hydrateDocument(hit)
			document.DataSources, err = decodeDataSources(hit.Fields, hit.ID)
			if err != nil {
				return err
			}
			link, _ := hit.Fields["DataSourceLink"].(string)
			count, _ := hit.Fields["DataSourceScoreCount"].(string)
			resource, _ := hit.Fields["GoodReadsResourceID"].(string)
			legacyID, _ := hit.Fields["GoodReadsLegacyBookID"].(float64)
			enriched, _ := hit.Fields["GoodReadsEnriched"].(bool)
			if link != "" || count != "" || resource != "" || legacyID != 0 || enriched {
				if _, exists := document.DataSources[biblioreads.Source]; !exists {
					if err := SetDataSourceData(&document, biblioreads.Source, biblioreads.DocumentData{
						URL: link, RatingCount: count, ResourceID: resource, LegacyBookID: int(legacyID), Enriched: enriched,
					}); err != nil {
						return err
					}
				}
			}
			if err := IndexDocument(batch, hit.ID, document); err != nil {
				return err
			}
		}
		if err := destination.Batch(batch); err != nil {
			return err
		}
	}
	for _, key := range [][]byte{internalLanguages, internalIllustratedMinSize, internalMinOccurrenceRatio, internalMaxTextRankWords, internalDocCountAtLastPrune} {
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
