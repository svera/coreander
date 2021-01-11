package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/blevesearch/bleve"
	"github.com/ilyakaznacheev/cleanenv"
	"github.com/svera/coreander/internal/index"
	"github.com/svera/coreander/internal/webserver"
	"github.com/svera/coreander/metadata"
)

func main() {
	//fastergoding.Run() // hot reload
	var cfg Config

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("Error retrieving user home dir")
	}
	if err = cleanenv.ReadConfig(homeDir+"/coreander/config.yml", &cfg); err != nil {
		log.Fatal(fmt.Sprintf("Config file config.yml not found in %s/coreander", homeDir))
	}
	/*
		if !cfg.Verbose {
			log.SetOutput(ioutil.Discard)
		}*/
	if _, err := os.Stat(cfg.LibraryPath); os.IsNotExist(err) {
		log.Fatal(fmt.Errorf("%s does not exist, exiting", cfg.LibraryPath))
	}
	run(cfg, homeDir)
}

func run(cfg Config, homeDir string) {
	var idx *index.BleveIndexer
	var err error
	idx, err = index.Open(homeDir)
	if err == bleve.ErrorIndexPathDoesNotExist {
		log.Println("No index found, creating a new one")
		idx, err = index.Create(homeDir)
		if err != nil {
			log.Fatal(err)
		}
	}
	go func() {
		start := time.Now().Unix()
		metadataReaders := map[string]metadata.Reader{
			".epub": metadata.Epub,
		}
		log.Println(fmt.Sprintf("Indexing books at %s, this can take a while depending on the size of your library.", cfg.LibraryPath))
		err := idx.Add(cfg.LibraryPath, metadataReaders, cfg.BatchSize)
		if err != nil {
			log.Fatal(err)
		}
		end := time.Now().Unix()
		dur, _ := time.ParseDuration(fmt.Sprintf("%ds", end-start))
		log.Println(fmt.Sprintf("Indexing finished, took %d seconds", int(dur.Seconds())))
	}()
	webserver.Start(idx, cfg.LibraryPath, cfg.Port)
}
