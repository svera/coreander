package index

import (
	"fmt"
	"log"
	"slices"

	"github.com/blevesearch/bleve/v2"
)

// MigrateAuthors migrates all authors from a legacy index to a new index.
// It searches only for items with Type = "author" and deletes them immediately after migration
// to avoid pagination issues and free up disk space.
func MigrateAuthors(oldIndex, newIndex bleve.Index, batchSize int) error {
	log.Println("Migrating authors from legacy index in batches...")

	// Create a query that filters for authors only
	typeQuery := bleve.NewTermQuery("author")
	typeQuery.SetField("Type")

	matchAllQuery := bleve.NewMatchAllQuery()
	conjunctionQuery := bleve.NewConjunctionQuery()
	conjunctionQuery.AddQuery(matchAllQuery)
	conjunctionQuery.AddQuery(typeQuery)

	totalMigrated := 0
	batchNumber := 0

	for {
		batchNumber++

		// Always query from the beginning (From = 0) to get the first batchSize authors
		// This avoids pagination issues when authors are deleted
		searchRequest := bleve.NewSearchRequest(conjunctionQuery)
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

		// Migrate authors in this batch
		authorsBatch := newIndex.NewBatch()
		deleteBatch := oldIndex.NewBatch()
		authorIDs := make([]string, 0)

		for _, hit := range searchResult.Hits {
			// Convert search result to Author
			author := hydrateAuthorFromFields(hit.Fields, hit.ID)
			if author.Slug == "" {
				continue
			}

			// Aggregate subject slugs from documents in the legacy index
			author.SubjectsSlugs = aggregateSubjectsForAuthorFromLegacyIndex(author.Slug, oldIndex)

			// Add author to new index batch
			if err := authorsBatch.Index(author.Slug, author); err != nil {
				return fmt.Errorf("error indexing author %s: %w", author.Slug, err)
			}

			// Add to delete batch
			deleteBatch.Delete(hit.ID)
			authorIDs = append(authorIDs, hit.ID)
		}

		// Commit authors to new index
		if authorsBatch.Size() > 0 {
			if err := newIndex.Batch(authorsBatch); err != nil {
				return fmt.Errorf("error committing batch: %w", err)
			}
		}

		// Delete authors from old index immediately
		if deleteBatch.Size() > 0 {
			if err := oldIndex.Batch(deleteBatch); err != nil {
				return fmt.Errorf("error deleting batch from legacy index: %w", err)
			}
		}

		totalMigrated += len(authorIDs)
		log.Printf("Migrated batch %d: %d authors (total migrated: %d)", batchNumber, len(authorIDs), totalMigrated)
	}

	log.Printf("Migration complete. Migrated %d authors total.", totalMigrated)
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

		// Migrate documents in this batch
		// After authors are migrated, the legacy index only contains documents
		documentsBatch := newIndex.NewBatch()
		deleteBatch := oldIndex.NewBatch()
		documentIDs := make([]string, 0)

		for _, hit := range searchResult.Hits {
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

// aggregateSubjectsForAuthorFromLegacyIndex collects all unique subject slugs from all documents by an author
// in the legacy index using Bleve faceted search for efficient aggregation.
func aggregateSubjectsForAuthorFromLegacyIndex(authorSlug string, legacyIndex bleve.Index) []string {
	if legacyIndex == nil {
		return []string{}
	}

	// Query for documents with this author slug in AuthorsSlugs field
	// Exclude authors by filtering out Type = "author"
	authorSlugQuery := bleve.NewTermQuery(authorSlug)
	authorSlugQuery.SetField("AuthorsSlugs")

	// Exclude authors from the query using a boolean query
	notAuthorQuery := bleve.NewBooleanQuery()
	typeQuery := bleve.NewTermQuery("author")
	typeQuery.SetField("Type")
	notAuthorQuery.AddMustNot(typeQuery)

	conjunctionQuery := bleve.NewConjunctionQuery()
	conjunctionQuery.AddQuery(authorSlugQuery)
	conjunctionQuery.AddQuery(notAuthorQuery)

	searchRequest := bleve.NewSearchRequest(conjunctionQuery)
	searchRequest.Size = 0 // We don't need document hits, only facets

	// Add facet request for SubjectsSlugs
	// Use a large size to get all unique terms
	subjectsSlugsFacet := bleve.NewFacetRequest("SubjectsSlugs", 10000)
	searchRequest.AddFacet("subjectsSlugs", subjectsSlugsFacet)

	searchResult, err := legacyIndex.Search(searchRequest)
	if err != nil {
		// Silently return empty subjects if query fails
		return []string{}
	}

	subjectsSlugs := []string{}

	// Extract subject slugs from facet results
	if subjectsSlugsFacetResult, ok := searchResult.Facets["subjectsSlugs"]; ok && subjectsSlugsFacetResult.Terms != nil {
		for _, term := range subjectsSlugsFacetResult.Terms.Terms() {
			if term.Term != "" {
				subjectsSlugs = append(subjectsSlugs, term.Term)
			}
		}
	}

	slices.Sort(subjectsSlugs)

	return subjectsSlugs
}
