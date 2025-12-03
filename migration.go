package main

import (
	"log"

	"github.com/blevesearch/bleve/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/index"
)

// migrateLegacyIndex migrates all data from a legacy single index to separate documents and authors indexes.
// Returns (needsReindex, migrationHappened) where:
// - needsReindex: true if reindexing is required (migration failed or index needs recreation)
// - migrationHappened: true if migration was attempted (regardless of success)
func migrateLegacyIndex(fs afero.Fs, homeDir, legacyIndexPath, documentsIndexPath, authorsIndexPath string) (bool, bool) {
	log.Println("Detected legacy single index format. Checking version...")

	// Open the legacy index
	legacyIndex, err := bleve.Open(homeDir + legacyIndexPath)
	if err != nil {
		log.Printf("Warning: Could not open legacy index: %v. Documents will be reindexed.", err)
		return true, false // Force reindexing, migration did not happen
	}
	legacyIndexClosed := false
	defer func() {
		if !legacyIndexClosed {
			legacyIndex.Close()
		}
	}()

	// Check the version of the legacy index
	legacyVersion, err := legacyIndex.GetInternal([]byte("version"))
	if err != nil {
		log.Printf("Warning: Could not read legacy index version: %v. Documents will be reindexed.", err)
		return true, false // Force reindexing, migration did not happen
	}
	legacyVersionStr := string(legacyVersion)

	// Only migrate authors if the legacy index version is v8
	if legacyVersionStr == "v8" {
		log.Println("Legacy index version is v8. Extracting authors before migration...")

		// Create authors index if it doesn't exist
		authorsIndexFullPath := homeDir + authorsIndexPath
		authorsIndexExists, _ := afero.DirExists(fs, authorsIndexFullPath)
		var authorsIndex bleve.Index

		if !authorsIndexExists {
			log.Println("Creating new authors index for migration...")
			authorsIndex = index.CreateAuthorsIndex(authorsIndexFullPath)
		} else {
			authorsIndex, err = bleve.Open(authorsIndexFullPath)
			if err != nil {
				log.Printf("Warning: Could not open authors index: %v. Authors will be reindexed.", err)
				return true, false // Force reindexing, migration did not happen
			}
		}
		defer authorsIndex.Close()

		// Extract authors from legacy index (filterForAuthorsOnly=true to skip documents)
		if err := index.MigrateAuthors(legacyIndex, authorsIndex, true); err != nil {
			log.Printf("Warning: Could not migrate authors from legacy index: %v. Authors will be reindexed.", err)
			return true, false // Force reindexing, migration did not happen
		}

		log.Println("Successfully extracted authors from legacy index.")
	} else {
		log.Printf("Legacy index version is %s (not v8). Authors will be reindexed.", legacyVersionStr)
	}

	// Create or open documents index for migration
	documentsIndexFullPath := homeDir + documentsIndexPath
	documentsIndexExists, _ := afero.DirExists(fs, documentsIndexFullPath)
	var documentsIndex bleve.Index

	if !documentsIndexExists {
		log.Println("Creating new documents index for migration...")
		documentsIndex = index.CreateDocumentsIndex(documentsIndexFullPath)
	} else {
		documentsIndex, err = bleve.Open(documentsIndexFullPath)
		if err != nil {
			log.Printf("Warning: Could not open documents index: %v. Documents will be reindexed.", err)
			return true, false // Force reindexing, migration did not happen
		}
	}
	defer documentsIndex.Close()

	// Migrate documents in batches from legacy index to new index
	log.Println("Migrating documents from legacy index in batches...")
	batchSize := 1000 // Use a reasonable batch size for migration
	if err := index.MigrateDocuments(legacyIndex, documentsIndex, batchSize); err != nil {
		log.Printf("Warning: Could not migrate documents from legacy index: %v. Documents will be reindexed.", err)
		return true, false // Force reindexing, migration did not happen
	}

	log.Println("Successfully migrated documents from legacy index.")

	// Check if legacy index still has any documents (it might only have authors left)
	// Search through all results to find any remaining documents
	matchAllQuery := bleve.NewMatchAllQuery()
	searchRequest := bleve.NewSearchRequest(matchAllQuery)
	searchRequest.Size = 1000 // Check in larger batches
	searchRequest.Fields = []string{"Title", "Name"}
	hasDocuments := false

	// Search through all results to find any remaining documents
	for {
		searchResult, err := legacyIndex.Search(searchRequest)
		if err != nil {
			log.Printf("Warning: Could not check legacy index contents: %v. Legacy index will be kept.", err)
			return false, true // Migration successful, don't force reindexing, migration happened
		}

		if searchResult.Total == 0 {
			break
		}

		// Check if there are any documents left (documents have Title field but not Name field)
		for _, hit := range searchResult.Hits {
			if hit.Fields["Title"] != nil && hit.Fields["Name"] == nil {
				hasDocuments = true
				break
			}
		}

		if hasDocuments {
			break
		}

		// If we got fewer hits than requested, we've checked all results
		if len(searchResult.Hits) < searchRequest.Size {
			break
		}

		// Move to next batch
		searchRequest.From += searchRequest.Size
	}

	// If no documents remain, remove the legacy index
	if !hasDocuments {
		log.Println("No documents remaining in legacy index. Removing legacy index...")
		if err := legacyIndex.Close(); err != nil {
			log.Printf("Warning: Could not close legacy index: %v", err)
		}
		legacyIndexClosed = true // Mark as closed so defer won't try to close it again
		if err := fs.RemoveAll(homeDir + legacyIndexPath); err != nil {
			log.Printf("Warning: Could not remove legacy index: %v", err)
		}
	} else {
		log.Println("Legacy index still contains documents. They will be migrated on next run.")
	}

	return false, true // Migration successful, don't force reindexing, migration happened
}

