package main

import (
	"log"

	"github.com/blevesearch/bleve/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/index"
)

// migrateLegacyIndex migrates all data from a legacy single index to separate documents and authors indexes.
// Returns needsReindex: true if reindexing is required (migration failed or index needs recreation),
// false if migration was successful and no reindexing is needed.
func migrateLegacyIndex(fs afero.Fs, homeDir, legacyIndexPath, documentsIndexPath, authorsIndexPath string) bool {
	log.Println("Detected legacy single index format. Checking version...")

	// Open the legacy index
	legacyIndex, err := bleve.Open(homeDir + legacyIndexPath)
	if err != nil {
		log.Printf("Warning: Could not open legacy index: %v. Documents will be reindexed.", err)
		return true // Force reindexing, migration did not happen
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
		return true // Force reindexing, migration did not happen
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
				return true // Force reindexing, migration did not happen
			}
		}
		defer authorsIndex.Close()

		// Extract authors from legacy index
		batchSize := 1000 // Use a reasonable batch size for migration
		if err := index.MigrateAuthors(legacyIndex, authorsIndex, batchSize); err != nil {
			log.Printf("Warning: Could not migrate authors from legacy index: %v. Authors will be reindexed.", err)
			return true // Force reindexing, migration did not happen
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
			return true // Force reindexing, migration did not happen
		}
	}
	defer documentsIndex.Close()

	// Migrate documents in batches from legacy index to new index
	log.Println("Migrating documents from legacy index in batches...")
	batchSize := 1000 // Use a reasonable batch size for migration
	if err := index.MigrateDocuments(legacyIndex, documentsIndex, batchSize); err != nil {
		log.Printf("Warning: Could not migrate documents from legacy index: %v. Documents will be reindexed.", err)
		return true // Force reindexing, migration did not happen
	}

	log.Println("Successfully migrated documents from legacy index.")

	// All documents have been migrated and deleted by MigrateDocuments
	// Remove the legacy index since migration is complete
	log.Println("Removing legacy index...")
	if err := legacyIndex.Close(); err != nil {
		log.Printf("Warning: Could not close legacy index: %v", err)
	}
	legacyIndexClosed = true // Mark as closed so defer won't try to close it again
	if err := fs.RemoveAll(homeDir + legacyIndexPath); err != nil {
		log.Printf("Warning: Could not remove legacy index: %v", err)
	}

	return false // Migration successful, don't force reindexing
}
