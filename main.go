package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/ilyakaznacheev/cleanenv"
	"github.com/spf13/afero"
	"github.com/svera/coreander/internal/index"
	"github.com/svera/coreander/internal/metadata"
	"github.com/svera/coreander/internal/webserver"
	"gopkg.in/fsnotify.v1"
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
	/*
		if !cfg.Verbose {
			log.SetOutput(ioutil.Discard)
		}*/
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
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}

	defer func() {
		watcher.Close()
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
		fileWatcher(watcher, idx, cfg.LibPath, metadataReaders)
	}()
	if err = watcher.Add(cfg.LibPath); err != nil {
		log.Fatal(err)
	}
	app := webserver.New(idx, cfg.LibPath)
	app.Listen(fmt.Sprintf(":%s", cfg.Port))
}

func fileWatcher(watcher *fsnotify.Watcher, idx *index.BleveIndexer, libPath string, readers map[string]metadata.Reader) {
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Create == fsnotify.Create {
				if err := idx.AddFile(event.Name); err != nil {
					log.Printf("Error indexing new file: %s\n", event.Name)
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Println("error:", err)
		}
	}
}
