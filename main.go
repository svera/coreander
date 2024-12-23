package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/ilyakaznacheev/cleanenv"
	"github.com/pirmd/epub"
	"gorm.io/gorm"

	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/metadata"
	"github.com/svera/coreander/v4/internal/webserver"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
)

var version string = "unknown"

const indexPath = "/.coreander/index"
const databasePath = "/.coreander/database.db"

var (
	cfg             Config
	appFs           afero.Fs
	idx             *index.BleveIndexer
	db              *gorm.DB
	homeDir         string
	err             error
	metadataReaders map[string]metadata.Reader
	sender          webserver.Sender
)

func init() {
	log.Printf("Coreander version %s starting\n", version)
	homeDir, err = os.UserHomeDir()
	if err != nil {
		log.Fatal("Error retrieving user home dir")
	}
	if err = cleanenv.ReadEnv(&cfg); err != nil {
		log.Fatalf("Error parsing configuration from environment variables: %s", err)
	}
	if _, err := os.Stat(cfg.LibPath); os.IsNotExist(err) {
		log.Fatalf("Directory '%s' does not exist, exiting", cfg.LibPath)
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
	idx = index.NewBleve(indexFile, appFs, cfg.LibPath, metadataReaders)
	db = infrastructure.Connect(homeDir+databasePath, cfg.WordsPerMinute)
}

func main() {
	defer idx.Close()

	go startIndex(idx, cfg.BatchSize, cfg.LibPath)

	sender = &infrastructure.NoEmail{}
	if cfg.SmtpServer != "" && cfg.SmtpUser != "" && cfg.SmtpPassword != "" {
		sender = &infrastructure.SMTP{
			Server:   cfg.SmtpServer,
			Port:     cfg.SmtpPort,
			User:     cfg.SmtpUser,
			Password: cfg.SmtpPassword,
		}
	}

	webserverConfig := webserver.Config{
		Version:               version,
		MinPasswordLength:     cfg.MinPasswordLength,
		WordsPerMinute:        cfg.WordsPerMinute,
		JwtSecret:             cfg.JwtSecret,
		FQDN:                  cfg.FQDN,
		Port:                  cfg.Port,
		HomeDir:               homeDir,
		LibraryPath:           cfg.LibPath,
		CoverMaxWidth:         cfg.CoverMaxWidth,
		RequireAuth:           cfg.RequireAuth,
		UploadDocumentMaxSize: cfg.UploadDocumentMaxSize,
	}

	webserverConfig.SessionTimeout, err = time.ParseDuration(fmt.Sprintf("%fh", cfg.SessionTimeout))
	if err != nil {
		log.Fatal(fmt.Errorf("wrong value for session timeout"))
	}

	webserverConfig.RecoveryTimeout, err = time.ParseDuration(fmt.Sprintf("%fh", cfg.RecoveryTimeout))
	if err != nil {
		log.Fatal(fmt.Errorf("wrong value for recovery timeout"))
	}

	controllers := webserver.SetupControllers(webserverConfig, db, metadataReaders, idx, sender, appFs)
	app := webserver.New(webserverConfig, controllers, sender, idx)
	if strings.ToLower(cfg.FQDN) == "localhost" {
		fmt.Printf("Warning: using \"localhost\" as FQDN. Links using this FQDN won't be accessible outside this system.\n")
	}
	log.Printf("Started listening on port %d\n", cfg.Port)
	log.Fatal(app.Listen(fmt.Sprintf(":%d", cfg.Port)))
}

func startIndex(idx *index.BleveIndexer, batchSize int, libPath string) {
	start := time.Now().Unix()
	log.Printf("Indexing documents at %s, this can take a while depending on the size of your library.", libPath)
	err := idx.AddLibrary(batchSize, cfg.ForceIndexing)
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
