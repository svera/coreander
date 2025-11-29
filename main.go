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
		log.Println("Old version authors index found, migrating to new mapping.")
		oldAuthorsIndex := authorsIndex
		oldIndexPath := homeDir + authorsIndexPath
		// Create temporary path for new index
		newIndexPath := homeDir + authorsIndexPath + "_new"
		// Create new index
		newAuthorsIndex := index.CreateAuthorsIndex(newIndexPath)
		// Migrate authors from old index to new index
		if err = index.MigrateAuthors(oldAuthorsIndex, newAuthorsIndex, false); err != nil {
			log.Printf("Warning: Could not migrate authors from old index: %v", err)
			needsReindex = true
			if err = newAuthorsIndex.Close(); err != nil {
				log.Fatal(err)
			}
			if err = fs.RemoveAll(newIndexPath); err != nil {
				log.Printf("Warning: Could not remove temporary index: %v", err)
			}
		} else {
			log.Println("Successfully migrated authors to new index.")
			// Close both indexes
			if err = oldAuthorsIndex.Close(); err != nil {
				log.Fatal(err)
			}
			if err = newAuthorsIndex.Close(); err != nil {
				log.Fatal(err)
			}
			// Remove old index
			if err = fs.RemoveAll(oldIndexPath); err != nil {
				log.Fatal(err)
			}
			// Rename new index to final path (use os.Rename since we're using OS filesystem)
			if err = os.Rename(newIndexPath, oldIndexPath); err != nil {
				log.Fatal(err)
			}
			// Reopen the migrated index
			authorsIndex, err = bleve.Open(oldIndexPath)
			if err != nil {
				log.Fatal(err)
			}
		}
	}

	return documentsIndex, authorsIndex, needsReindex
}

func migrateLegacyIndex(fs afero.Fs) bool {
	log.Println("Detected legacy single index format. Extracting authors before migration...")

	// Open the legacy index
	legacyIndex, err := bleve.Open(homeDir + legacyIndexPath)
	if err != nil {
		log.Printf("Warning: Could not open legacy index: %v. Authors will be reindexed.", err)
		return true // Force reindexing
	}
	defer legacyIndex.Close()

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

	// Now remove the legacy index
	log.Println("Removing legacy single index. Documents will be reindexed.")
	if err := fs.RemoveAll(homeDir + legacyIndexPath); err != nil {
		log.Printf("Warning: Could not remove legacy index: %v", err)
	}
	return true // Force reindexing documents
}
