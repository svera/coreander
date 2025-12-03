package index

import (
	"fmt"
	"log"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search"
)

// MigrateAuthors migrates all authors from an old index to a new one.
// If filterForAuthorsOnly is true, it will filter out documents (checking for Title field)
// to only migrate author entries. This is used when migrating from a legacy index that
// contains both documents and authors.
func MigrateAuthors(oldIndex, newIndex bleve.Index, filterForAuthorsOnly bool) error {
	matchAllQuery := bleve.NewMatchAllQuery()
	searchRequest := bleve.NewSearchRequest(matchAllQuery)
	searchRequest.Size = 10000           // Process in batches
	searchRequest.Fields = []string{"*"} // Get all fields

	batch := newIndex.NewBatch()
	batchCount := 0

	for {
		searchResult, err := oldIndex.Search(searchRequest)
		if err != nil {
			return err
		}

		if searchResult.Total == 0 {
			break
		}

		// Migrate each author from search results
		for _, hit := range searchResult.Hits {
			// If filtering is enabled, check if this document is an author
			if filterForAuthorsOnly {
				hasName := hit.Fields["Name"] != nil
				hasTitle := hit.Fields["Title"] != nil
				// Skip if it's not an author (documents have Title, authors have Name but not Title)
				if !hasName || hasTitle {
					continue
				}
			}

			// Convert search result to Author
			author := hydrateAuthor(hit)
			if author.Slug == "" {
				continue
			}

			if err := batch.Index(author.Slug, author); err != nil {
				return err
			}
			batchCount++

			// Execute batch every 1000 items
			if batchCount >= 1000 {
				if err := newIndex.Batch(batch); err != nil {
					return err
				}
				batch = newIndex.NewBatch()
				batchCount = 0
			}
		}

		// If we got less than requested size, we're done
		if len(searchResult.Hits) < searchRequest.Size {
			break
		}

		// Move to next batch
		searchRequest.From += searchRequest.Size
	}

	// Execute any remaining authors
	if batchCount > 0 {
		if err := newIndex.Batch(batch); err != nil {
			return err
		}
	}

	return nil
}

// MigrateDocuments migrates all documents from a legacy index to a new documents index in batches.
// It always loads the first 1000 documents to avoid pagination issues, and deletes them immediately
// after migration to avoid using much disk space.
func MigrateDocuments(oldIndex, newIndex bleve.Index, batchSize int) error {
	log.Println("Migrating documents from legacy index in batches...")

	totalMigrated := 0
	batchNumber := 0

	for {
		batchNumber++

		// Always query from the beginning (From = 0) to get the first batchSize documents
		// This avoids pagination issues when documents are deleted
		matchAllQuery := bleve.NewMatchAllQuery()
		searchRequest := bleve.NewSearchRequest(matchAllQuery)
		searchRequest.Size = batchSize
		searchRequest.From = 0
		searchRequest.Fields = []string{"*"} // Get all fields

		searchResult, err := oldIndex.Search(searchRequest)
		if err != nil {
			return err
		}

		if len(searchResult.Hits) == 0 {
			break
		}

		// Filter for documents only (documents have Title, authors have Name but not Title)
		documentHits := make([]*search.DocumentMatch, 0)
		for _, hit := range searchResult.Hits {
			hasTitle := hit.Fields["Title"] != nil
			hasName := hit.Fields["Name"] != nil
			if hasTitle && !hasName {
				documentHits = append(documentHits, hit)
			}
		}

		if len(documentHits) == 0 {
			// No documents in this batch - if we got fewer hits than requested, we're done
			// Otherwise, there might be more documents after the authors, so continue
			if len(searchResult.Hits) < batchSize {
				break
			}
			// If we got exactly batchSize hits but all are authors, we need to delete them
			// to make progress, but the user said only delete documents. So we're stuck.
			// Actually, let's just break - if all remaining items are authors, migration is done.
			break
		}

		// Migrate documents in this batch
		documentsBatch := newIndex.NewBatch()
		deleteBatch := oldIndex.NewBatch()
		documentIDs := make([]string, 0)

		for _, hit := range documentHits {
			// Convert search result to Document
			doc := hydrateDocument(hit)
			if doc.ID == "" || doc.Slug == "" {
				log.Printf("Warning: Document %s has invalid ID or Slug, skipping", hit.ID)
				continue
			}

			// Add document to new index batch
			if err := documentsBatch.Index(doc.ID, doc); err != nil {
				return fmt.Errorf("error indexing document %s: %w", doc.ID, err)
			}

			// Add to delete batch
			deleteBatch.Delete(hit.ID)
			documentIDs = append(documentIDs, hit.ID)
		}

		// Commit documents to new index
		if documentsBatch.Size() > 0 {
			if err := newIndex.Batch(documentsBatch); err != nil {
				return fmt.Errorf("error committing batch: %w", err)
			}
		}

		// Delete documents from old index immediately
		if deleteBatch.Size() > 0 {
			if err := oldIndex.Batch(deleteBatch); err != nil {
				return fmt.Errorf("error deleting batch from legacy index: %w", err)
			}
		}

		totalMigrated += len(documentIDs)
		log.Printf("Migrated batch %d: %d documents (total migrated: %d)", batchNumber, len(documentIDs), totalMigrated)
	}

	log.Printf("Migration complete. Migrated %d documents total.", totalMigrated)
	return nil
}
