package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/pirmd/epub"
	"gorm.io/gorm"

	"github.com/alecthomas/kong"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/datasource/wikidata"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/metadata"
	"github.com/svera/coreander/v4/internal/webserver"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
)

var version string = "unknown"

const documentsIndexPath = "/.coreander/documents_index"
const authorsIndexPath = "/.coreander/authors_index"
const legacyIndexPath = "/.coreander/index" // Old single index path
const databasePath = "/.coreander/database.db"

var (
	input           CLIInput
	appFs           afero.Fs
	idx             *index.BleveIndexer
	db              *gorm.DB
	homeDir         string
	err             error
	metadataReaders map[string]metadata.Reader
	sender          webserver.Sender
)

func init() {
	ctx := kong.Parse(&input, kong.Description(`
		Coreander is a document management system which indexes metadata from documents in a library and allows users to search and read them through a web interface.
	`),
		kong.Vars{
			"version": version,
		},
	)

	if ctx.Error != nil {
		log.Fatalf("Error parsing configuration: %s", ctx.Error)
	}

	log.Printf("Coreander version %s starting\n", version)
	homeDir, err = os.UserHomeDir()
	if err != nil {
		log.Fatal("Error retrieving user home dir")
	}

	if _, err := os.Stat(input.LibPath); os.IsNotExist(err) {
		log.Fatalf("Directory '%s' does not exist, exiting", input.LibPath)
	}

	metadataReaders = map[string]metadata.Reader{
		".epub": metadata.EpubReader{
			GetMetadataFromFile: epub.GetMetadataFromFile,
			GetPackageFromFile:  epub.GetPackageFromFile,
		},
		".pdf": metadata.PdfReader{},
	}

	appFs = afero.NewOsFs()

	documentsIndex, authorsIndex, needsReindex := getIndexes(appFs)
	idx = index.NewBleve(documentsIndex, authorsIndex, appFs, input.LibPath, metadataReaders)

	// If index was migrated or newly created, force reindexing
	if needsReindex {
		input.ForceIndexing = true
	}
	db = infrastructure.Connect(homeDir+databasePath, input.WordsPerMinute)
}

func main() {
	defer idx.Close()

	go startIndex(idx, input.BatchSize, input.LibPath)

	sender = &infrastructure.NoEmail{}
	if input.SmtpServer != "" && input.SmtpUser != "" && input.SmtpPassword != "" {
		sender = &infrastructure.SMTP{
			Server:   input.SmtpServer,
			Port:     input.SmtpPort,
			User:     input.SmtpUser,
			Password: input.SmtpPassword,
		}
	}

	webserverConfig := webserver.Config{
		Version:                    version,
		MinPasswordLength:          input.MinPasswordLength,
		WordsPerMinute:             input.WordsPerMinute,
		JwtSecret:                  []byte(input.JwtSecret),
		FQDN:                       input.FQDN,
		Port:                       input.Port,
		HomeDir:                    homeDir,
		CacheDir:                   input.CacheDir,
		LibraryPath:                input.LibPath,
		AuthorImageMaxWidth:        input.AuthorImageMaxWidth,
		CoverMaxWidth:              input.CoverMaxWidth,
		RequireAuth:                input.RequireAuth,
		UploadDocumentMaxSize:      input.UploadDocumentMaxSize,
		ClientStaticCacheTTL:       input.ClientStaticCacheTTL,
		ClientDynamicImageCacheTTL: input.ClientDynamicImageCacheTTL,
		ServerStaticCacheTTL:       input.ServerStaticCacheTTL,
		ServerDynamicImageCacheTTL: input.ServerDynamicImageCacheTTL,
	}

	webserverConfig.SessionTimeout, err = time.ParseDuration(fmt.Sprintf("%fh", input.SessionTimeout))
	if err != nil {
		log.Fatal(fmt.Errorf("wrong value for session timeout"))
	}

	webserverConfig.RecoveryTimeout, err = time.ParseDuration(fmt.Sprintf("%fh", input.RecoveryTimeout))
	if err != nil {
		log.Fatal(fmt.Errorf("wrong value for recovery timeout"))
	}

	webserverConfig.InvitationTimeout, err = time.ParseDuration(fmt.Sprintf("%fh", input.InvitationTimeout))
	if err != nil {
		log.Fatal(fmt.Errorf("wrong value for invitation timeout"))
	}

	if webserverConfig.CacheDir == "" {
		webserverConfig.CacheDir = homeDir + "/.coreander/cache"
		if _, err := os.Stat(webserverConfig.CacheDir); os.IsNotExist(err) {
			if err = os.MkdirAll(webserverConfig.CacheDir, os.ModePerm); err != nil {
				log.Fatal(err)
			}
			log.Printf("Created cache folder at %s\n", webserverConfig.CacheDir)
		}
	}

	dataSource := wikidata.NewWikidataSource(wikidata.Gowikidata{})

	controllers := webserver.SetupControllers(webserverConfig, db, metadataReaders, idx, sender, appFs, dataSource)
	app := webserver.New(webserverConfig, controllers, sender, idx)
	if strings.ToLower(input.FQDN) == "localhost" {
		fmt.Printf("Warning: using \"localhost\" as FQDN. Links using this FQDN won't be accessible outside this system.\n")
	}
	log.Printf("Started listening on port %d\n", input.Port)
	log.Fatal(app.Listen(fmt.Sprintf(":%d", input.Port)))
}

func startIndex(idx *index.BleveIndexer, batchSize int, libPath string) {
	start := time.Now().Unix()
	log.Printf("Indexing documents at %s, this can take a while depending on the size of your library.", libPath)
	err := idx.AddLibrary(batchSize, input.ForceIndexing)
	if err != nil {
		log.Fatal(err)
	}
	end := time.Now().Unix()
	dur, _ := time.ParseDuration(fmt.Sprintf("%ds", end-start))
	log.Printf("Indexing finished, took %d seconds", int(dur.Seconds()))
	fileWatcher(idx, libPath)
}

func getIndexes(fs afero.Fs) (bleve.Index, bleve.Index, bool) {
	needsReindex := false

	// Check if legacy single index exists (migration scenario)
	legacyExists, _ := afero.DirExists(fs, homeDir+legacyIndexPath)
	if legacyExists {
		log.Println("Detected legacy single index format. Migrating to separate indexes...")
		needsReindex = migrateLegacyIndex(fs)
	}

	// Open or create documents index
	documentsIndex, err := bleve.Open(homeDir + documentsIndexPath)
	if err == bleve.ErrorIndexPathDoesNotExist {
		log.Println("No documents index found, creating a new one.")
		documentsIndex = index.CreateDocumentsIndex(homeDir + documentsIndexPath)
		needsReindex = true
	}

	// Open or create authors index
	authorsIndex, err := bleve.Open(homeDir + authorsIndexPath)
	if err == bleve.ErrorIndexPathDoesNotExist {
		log.Println("No authors index found, creating a new one.")
		authorsIndex = index.CreateAuthorsIndex(homeDir + authorsIndexPath)
		needsReindex = true
	}

	// Check documents index version
	version, err := documentsIndex.GetInternal([]byte("version"))
	if err != nil {
		log.Fatal(err)
	}
	if string(version) == "" || string(version) < index.DocumentVersion {
		log.Println("Old version documents index found, recreating with new mapping.")
		if err = documentsIndex.Close(); err != nil {
			log.Fatal(err)
		}
		if err = fs.RemoveAll(homeDir + documentsIndexPath); err != nil {
			log.Fatal(err)
		}
		documentsIndex = index.CreateDocumentsIndex(homeDir + documentsIndexPath)
		needsReindex = true
	}

	// Check authors index version
	version, err = authorsIndex.GetInternal([]byte("version"))
	if err != nil {
		log.Fatal(err)
	}
	if string(version) == "" || string(version) < index.AuthorVersion {
		log.Println("Old version authors index found, recreating with new mapping.")
		if err = authorsIndex.Close(); err != nil {
			log.Fatal(err)
		}
		if err = fs.RemoveAll(homeDir + authorsIndexPath); err != nil {
			log.Fatal(err)
		}
		authorsIndex = index.CreateAuthorsIndex(homeDir + authorsIndexPath)
		needsReindex = true
	}

	return documentsIndex, authorsIndex, needsReindex
}

func migrateLegacyIndex(fs afero.Fs) bool {
	log.Println("Detected legacy single index format. Checking version...")

	// Open the legacy index
	legacyIndex, err := bleve.Open(homeDir + legacyIndexPath)
	if err != nil {
		log.Printf("Warning: Could not open legacy index: %v. Documents will be reindexed.", err)
		return true // Force reindexing
	}
	defer legacyIndex.Close()

	// Check the version of the legacy index
	legacyVersion, err := legacyIndex.GetInternal([]byte("version"))
	if err != nil {
		log.Printf("Warning: Could not read legacy index version: %v. Documents will be reindexed.", err)
		return true // Force reindexing
	}
	legacyVersionStr := string(legacyVersion)

	// Only migrate authors if the legacy index version is v8
	if legacyVersionStr == "v8" {
		log.Println("Legacy index version is v8. Extracting authors before migration...")

		// Create authors index if it doesn't exist
		authorsIndexPath := homeDir + authorsIndexPath
		authorsIndexExists, _ := afero.DirExists(fs, authorsIndexPath)
		var authorsIndex bleve.Index

		if !authorsIndexExists {
			log.Println("Creating new authors index for migration...")
			authorsIndex = index.CreateAuthorsIndex(authorsIndexPath)
		} else {
			authorsIndex, err = bleve.Open(authorsIndexPath)
			if err != nil {
				log.Printf("Warning: Could not open authors index: %v. Authors will be reindexed.", err)
				return true // Force reindexing
			}
		}
		defer authorsIndex.Close()

		// Extract authors from legacy index (filterForAuthorsOnly=true to skip documents)
		if err := index.MigrateAuthors(legacyIndex, authorsIndex, true); err != nil {
			log.Printf("Warning: Could not migrate authors from legacy index: %v. Authors will be reindexed.", err)
			return true // Force reindexing
		}

		log.Println("Successfully extracted authors from legacy index.")
	} else {
		log.Printf("Legacy index version is %s (not v8). Authors will be reindexed.", legacyVersionStr)
	}

	// Create or open documents index for migration
	documentsIndexPath := homeDir + documentsIndexPath
	documentsIndexExists, _ := afero.DirExists(fs, documentsIndexPath)
	var documentsIndex bleve.Index

	if !documentsIndexExists {
		log.Println("Creating new documents index for migration...")
		documentsIndex = index.CreateDocumentsIndex(documentsIndexPath)
	} else {
		documentsIndex, err = bleve.Open(documentsIndexPath)
		if err != nil {
			log.Printf("Warning: Could not open documents index: %v. Documents will be reindexed.", err)
			return true // Force reindexing
		}
	}
	defer documentsIndex.Close()

	// Migrate documents in batches from legacy index to new index
	log.Println("Migrating documents from legacy index in batches...")
	batchSize := 1000 // Use a reasonable batch size for migration
	if err := index.MigrateDocuments(legacyIndex, documentsIndex, batchSize); err != nil {
		log.Printf("Warning: Could not migrate documents from legacy index: %v. Documents will be reindexed.", err)
		return true // Force reindexing
	}

	log.Println("Successfully migrated documents from legacy index.")

	// Check if legacy index still has any documents (it might only have authors left)
	// Search for documents specifically by looking for entries with Title field but no Name field
	matchAllQuery := bleve.NewMatchAllQuery()
	searchRequest := bleve.NewSearchRequest(matchAllQuery)
	searchRequest.Size = 100 // Check a reasonable sample to see if any documents remain
	searchRequest.Fields = []string{"Title", "Name"}
	searchResult, err := legacyIndex.Search(searchRequest)
	if err != nil {
		log.Printf("Warning: Could not check legacy index contents: %v. Legacy index will be kept.", err)
		return false // Migration successful, don't force reindexing
	}

	// Check if there are any documents left (documents have Title field but not Name field)
	hasDocuments := false
	for _, hit := range searchResult.Hits {
		if hit.Fields["Title"] != nil && hit.Fields["Name"] == nil {
			hasDocuments = true
			break
		}
	}

	// If no documents remain, remove the legacy index
	if !hasDocuments {
		log.Println("No documents remaining in legacy index. Removing legacy index...")
		if err := legacyIndex.Close(); err != nil {
			log.Printf("Warning: Could not close legacy index: %v", err)
		}
		if err := fs.RemoveAll(homeDir + legacyIndexPath); err != nil {
			log.Printf("Warning: Could not remove legacy index: %v", err)
		}
	} else {
		log.Println("Legacy index still contains documents. They will be migrated on next run.")
	}

	return false // Migration successful, don't force reindexing
}
