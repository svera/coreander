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

const indexPath = "/.coreander/index"
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

	indexFile := getIndexFile(appFs)
	idx = index.NewBleve(indexFile, appFs, input.LibPath, metadataReaders)
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

func getIndexFile(fs afero.Fs) bleve.Index {
	indexFile, err := bleve.Open(homeDir + indexPath)
	if err == bleve.ErrorIndexPathDoesNotExist {
		log.Println("No index found, creating a new one.")
		indexFile = index.Create(homeDir + indexPath)
	}
	version, err := indexFile.GetInternal([]byte("version"))
	if err != nil {
		log.Fatal(err)
	}
	if string(version) == "" || string(version) < index.Version {
		log.Println("Old version index found, recreating it.")
		if err = fs.RemoveAll(homeDir + indexPath); err != nil {
			log.Fatal(err)
		}
		indexFile = index.Create(homeDir + indexPath)
	}
	return indexFile
}
