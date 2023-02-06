package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/ilyakaznacheev/cleanenv"
	"gorm.io/gorm"

	"github.com/spf13/afero"
	"github.com/svera/coreander/internal/controller"
	"github.com/svera/coreander/internal/index"
	"github.com/svera/coreander/internal/infrastructure"
	"github.com/svera/coreander/internal/metadata"
	"github.com/svera/coreander/internal/webserver"
)

var version string = "unknown"

func main() {
	var (
		cfg   Config
		appFs = afero.NewOsFs()
		idx   *index.BleveIndexer
	)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("Error retrieving user home dir")
	}
	if err = cleanenv.ReadEnv(&cfg); err != nil {
		log.Fatal(fmt.Sprintf("Error parsing configuration from environment variables: %s", err))
	}
	if _, err := os.Stat(cfg.LibPath); os.IsNotExist(err) {
		log.Fatal(fmt.Errorf("Directory '%s' does not exist, exiting", cfg.LibPath))
	}

	metadataReaders := map[string]metadata.Reader{
		".epub": metadata.EpubReader{},
	}

	indexFile, err := bleve.Open(homeDir + "/coreander/db")
	if err == nil {
		idx = index.NewBleve(indexFile, cfg.LibPath, metadataReaders)
	}
	if err == bleve.ErrorIndexPathDoesNotExist {
		cfg.SkipReindex = false
		idx = createIndex(homeDir, cfg.LibPath, metadataReaders)
	}
	db := infrastructure.Connect(homeDir + "/coreander/db/database.db")

	run(cfg, db, idx, homeDir, metadataReaders, appFs)
}

func run(cfg Config, db *gorm.DB, idx *index.BleveIndexer, homeDir string, metadataReaders map[string]metadata.Reader, appFs afero.Fs) {
	var (
		err    error
		sender controller.Sender
	)

	defer idx.Close()

	if !cfg.SkipReindex {
		go reindex(idx, appFs, cfg.BatchSize, cfg.LibPath)
	}
	sender = &infrastructure.NoEmail{}
	if cfg.SmtpServer != "" && cfg.SmtpUser != "" && cfg.SmtpPassword != "" {
		sender = &infrastructure.SMTP{
			Server:   cfg.SmtpServer,
			Port:     cfg.SmtpPort,
			User:     cfg.SmtpUser,
			Password: cfg.SmtpPassword,
		}
	}
	app := webserver.New(idx, cfg.LibPath, homeDir, version, metadataReaders, cfg.CoverMaxWidth, sender, db)
	fmt.Printf("Coreander version %s started listening on port %s\n\n", version, cfg.Port)
	err = app.Listen(fmt.Sprintf(":%s", cfg.Port))
	if err != nil {
		log.Fatal(err)
	}
}

func reindex(idx *index.BleveIndexer, appFs afero.Fs, batchSize int, libPath string) {
	start := time.Now().Unix()
	log.Println(fmt.Sprintf("Indexing books at %s, this can take a while depending on the size of your library.", libPath))
	err := idx.AddLibrary(appFs, batchSize)
	if err != nil {
		log.Fatal(err)
	}
	end := time.Now().Unix()
	dur, _ := time.ParseDuration(fmt.Sprintf("%ds", end-start))
	log.Println(fmt.Sprintf("Indexing finished, took %d seconds", int(dur.Seconds())))
	fileWatcher(idx, libPath)
}

func createIndex(homeDir, libPath string, metadataReaders map[string]metadata.Reader) *index.BleveIndexer {
	log.Println("No index found, creating a new one")
	indexMapping := bleve.NewIndexMapping()
	index.AddMappings(indexMapping)
	indexFile, err := bleve.New(homeDir+"/coreander/db", indexMapping)
	if err != nil {
		log.Fatal(err)
	}
	return index.NewBleve(indexFile, libPath, metadataReaders)
}
