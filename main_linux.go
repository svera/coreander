package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/ilyakaznacheev/cleanenv"
	"github.com/rjeczalik/notify"
	"github.com/spf13/afero"
	"github.com/svera/coreander/internal/index"
	"github.com/svera/coreander/internal/metadata"
	"github.com/svera/coreander/internal/webserver"
)

func main() {
	//fastergoding.Run() // hot reload
	var cfg Config

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
		".epub": metadata.Epub,
	}

	run(cfg, homeDir, metadataReaders)
}

func run(cfg Config, homeDir string, metadataReaders map[string]metadata.Reader) {
	var idx *index.BleveIndexer
	var err error
	indexFile, err := bleve.Open(homeDir + "/coreander/db")
	if err == nil {
		idx = index.NewBleve(indexFile, cfg.LibPath, metadataReaders)
	}
	if err == bleve.ErrorIndexPathDoesNotExist {
		log.Println("No index found, creating a new one")
		indexMapping := bleve.NewIndexMapping()
		index.AddLanguageMappings(indexMapping)
		indexFile, err := bleve.New(homeDir+"/coreander/db", indexMapping)
		if err != nil {
			log.Fatal(err)
		}
		idx = index.NewBleve(indexFile, cfg.LibPath, metadataReaders)
	}
	c := make(chan notify.EventInfo, 1)
	if err := notify.Watch(cfg.LibPath, c, notify.InCloseWrite, notify.InMovedTo); err != nil {
		log.Fatal(err)
	}
	defer func() {
		notify.Stop(c)
		idx.Close()
	}()

	go func() {
		start := time.Now().Unix()
		var appFs = afero.NewOsFs()
		log.Println(fmt.Sprintf("Indexing books at %s, this can take a while depending on the size of your library.", cfg.LibPath))
		err := idx.AddLibrary(appFs, cfg.BatchSize)
		if err != nil {
			log.Fatal(err)
		}
		end := time.Now().Unix()
		dur, _ := time.ParseDuration(fmt.Sprintf("%ds", end-start))
		log.Println(fmt.Sprintf("Indexing finished, took %d seconds", int(dur.Seconds())))
		log.Printf("Starting file watcher on %s\n", cfg.LibPath)
		fileWatcher(c, idx, cfg.LibPath, metadataReaders)
	}()
	app := webserver.New(idx, cfg.LibPath)
	app.Listen(fmt.Sprintf(":%s", cfg.Port))
}

func fileWatcher(c <-chan (notify.EventInfo), idx *index.BleveIndexer, libPath string, readers map[string]metadata.Reader) {
	for {
		select {
		case ei := <-c:
			if err := idx.AddFile(ei.Path()); err != nil {
				log.Printf("Error indexing new file: %s\n", ei.Path())
			}
		}
	}
}
