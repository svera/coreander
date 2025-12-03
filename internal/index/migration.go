package index

import (
	"fmt"
	"log"

	"github.com/blevesearch/bleve/v2"
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
// After successfully migrating all documents, they are removed from the legacy index.
// This approach avoids pagination issues that occur when deleting documents during migration.
func MigrateDocuments(oldIndex, newIndex bleve.Index, batchSize int) error {
	log.Println("Step 1: Collecting all document IDs from legacy index...")

	// First pass: collect all document IDs that need to be migrated
	// We don't delete during this pass to avoid pagination issues
	matchAllQuery := bleve.NewMatchAllQuery()
	searchRequest := bleve.NewSearchRequest(matchAllQuery)
	searchRequest.Size = batchSize
	searchRequest.Fields = []string{"Title", "Name"} // Only need these fields for filtering

	documentIDs := make([]string, 0)

	batchNumber := 0
	for {
		batchNumber++
		searchResult, err := oldIndex.Search(searchRequest)
		if err != nil {
			return err
		}

		if len(searchResult.Hits) == 0 {
			break
		}

		// Collect document IDs and data
		for _, hit := range searchResult.Hits {
			// Filter for documents only (documents have Title, authors have Name but not Title)
			hasTitle := hit.Fields["Title"] != nil
			hasName := hit.Fields["Name"] != nil
			if hasTitle && !hasName {
				documentIDs = append(documentIDs, hit.ID)
			}
		}

		if batchNumber%10 == 0 {
			log.Printf("Collection batch %d: found %d documents so far (total hits: %d)",
				batchNumber, len(documentIDs), len(searchResult.Hits))
		}

		// Continue until we get 0 hits
		if len(searchResult.Hits) < searchRequest.Size {
			break
		}

		searchRequest.From += searchRequest.Size
	}

	log.Printf("Step 2: Collected %d documents. Now fetching full document data and migrating...", len(documentIDs))

	// Second pass: fetch full document data and migrate in batches
	// We need to fetch the full document data now since we only stored Title/Name before
	searchRequest.Fields = []string{"*"} // Get all fields
	documentsBatch := newIndex.NewBatch()
	documentsBatchCount := 0
	totalMigrated := 0

	for _, docID := range documentIDs {
		// Fetch the full document
		docIDQuery := bleve.NewDocIDQuery([]string{docID})
		fetchRequest := bleve.NewSearchRequest(docIDQuery)
		fetchRequest.Fields = []string{"*"}
		fetchRequest.Size = 1

		fetchResult, err := oldIndex.Search(fetchRequest)
		if err != nil {
			log.Printf("Warning: Could not fetch document %s: %v", docID, err)
			continue
		}

		if len(fetchResult.Hits) == 0 {
			log.Printf("Warning: Document %s not found in legacy index", docID)
			continue
		}

		hit := fetchResult.Hits[0]

		// Convert search result to Document
		doc := hydrateDocument(hit)
		if doc.ID == "" || doc.Slug == "" {
			log.Printf("Warning: Document %s has invalid ID or Slug, skipping", docID)
			continue
		}

		// Add document to new index batch
		if err := documentsBatch.Index(doc.ID, doc); err != nil {
			return fmt.Errorf("error indexing document %s: %w", docID, err)
		}
		documentsBatchCount++
		totalMigrated++

		// Execute batch when it reaches the specified size
		if documentsBatchCount >= batchSize {
			if err := newIndex.Batch(documentsBatch); err != nil {
				return fmt.Errorf("error committing batch: %w", err)
			}
			documentsBatch = newIndex.NewBatch()
			documentsBatchCount = 0
			log.Printf("Migrated batch: %d/%d documents", totalMigrated, len(documentIDs))
		}
	}

	// Execute any remaining documents
	if documentsBatchCount > 0 {
		if err := newIndex.Batch(documentsBatch); err != nil {
			return fmt.Errorf("error committing final batch: %w", err)
		}
		log.Printf("Migrated final batch: %d/%d documents", totalMigrated, len(documentIDs))
	}

	log.Printf("Step 3: Migration complete. Migrated %d documents. Now deleting from legacy index...", totalMigrated)

	// Third pass: delete all migrated documents from old index
	deleteBatch := oldIndex.NewBatch()
	deleteCount := 0
	for _, docID := range documentIDs {
		deleteBatch.Delete(docID)
		deleteCount++

		if deleteCount >= batchSize {
			if err := oldIndex.Batch(deleteBatch); err != nil {
				return fmt.Errorf("error deleting batch from legacy index: %w", err)
			}
			deleteBatch = oldIndex.NewBatch()
			deleteCount = 0
		}
	}

	// Delete remaining documents
	if deleteCount > 0 {
		if err := oldIndex.Batch(deleteBatch); err != nil {
			return fmt.Errorf("error deleting final batch from legacy index: %w", err)
		}
	}

	log.Printf("Step 4: Deleted %d documents from legacy index. Migration fully complete.", len(documentIDs))
	return nil
}

