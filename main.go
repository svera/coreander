package main

import (
	"fmt"
	"log"
	"os"

	"github.com/blevesearch/bleve"
	"github.com/ilyakaznacheev/cleanenv"
	"github.com/qinains/fastergoding"
	"github.com/svera/coreander/internal/index"
	"github.com/svera/coreander/internal/webserver"
)

func main() {
	fastergoding.Run() // hot reload
	var cfg Config

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("Error retrieving user home dir")
	}
	if err = cleanenv.ReadConfig(homeDir+"/coreander/config.yml", &cfg); err != nil {
		log.Fatal(fmt.Sprintf("Config file config.yml not found in %s/coreander", homeDir))
	}
	if _, err := os.Stat(cfg.LibraryPath); os.IsNotExist(err) {
		log.Fatal(fmt.Errorf("%s does not exist, exiting", cfg.LibraryPath))
	}
	var idx *index.BleveIndexer
	idx, err = index.Open(homeDir)
	if err == bleve.ErrorIndexPathDoesNotExist {
		log.Println("No index found, creating a new one")
		idx, err = index.Create(homeDir)
		if err != nil {
			log.Fatal(err)
		}
		go func() {
			log.Println(fmt.Sprintf("Indexing books at %s, this can take a while depending on the size of your library.", cfg.LibraryPath))
			err := idx.Add(cfg.LibraryPath, cfg.BatchSize)
			if err != nil {
				log.Fatal(err)
			}
		}()
	}
	webserver.Start(idx, cfg.LibraryPath, cfg.Port)
}
