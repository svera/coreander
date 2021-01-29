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
	run(cfg, homeDir)
}

func run(cfg Config, homeDir string) {
	var idx *index.BleveIndexer
	var err error
	indexFile, err := bleve.Open(homeDir + "/coreander/db")
	if err == nil {
		idx = index.NewBleve(indexFile)
	}
	if err == bleve.ErrorIndexPathDoesNotExist {
		log.Println("No index found, creating a new one")
		idx, err = index.CreateBleve(homeDir)
		if err != nil {
			log.Fatal(err)
		}
	}

	defer idx.Close()

	go func() {
		start := time.Now().Unix()
		var appFs = afero.NewOsFs()
		metadataReaders := map[string]metadata.Reader{
			".epub": metadata.Epub,
		}
		log.Println(fmt.Sprintf("Indexing books at %s, this can take a while depending on the size of your library.", cfg.LibPath))
		err := idx.Add(cfg.LibPath, appFs, metadataReaders, cfg.BatchSize)
		if err != nil {
			log.Fatal(err)
		}
		end := time.Now().Unix()
		dur, _ := time.ParseDuration(fmt.Sprintf("%ds", end-start))
		log.Println(fmt.Sprintf("Indexing finished, took %d seconds", int(dur.Seconds())))
	}()
	app := webserver.New(idx, cfg.LibPath)
	app.Listen(fmt.Sprintf(":%s", cfg.Port))
}
