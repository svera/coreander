package main

import (
	"fmt"
	"log"
	"os"

	"github.com/blevesearch/bleve"
	"github.com/ilyakaznacheev/cleanenv"
	"github.com/qinains/fastergoding"
	"github.com/svera/coreander/config"
	"github.com/svera/coreander/indexer"
	"github.com/svera/coreander/webserver"
)

const resultsPerPage = 10
const maxPagesNavigator = 10

func main() {
	fastergoding.Run() // hot reload
	var cfg config.Config

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("Error retrieving user home dir")
	}
	if err = cleanenv.ReadConfig(homeDir+"/coreander/coreander.yml", &cfg); err != nil {
		log.Fatal(fmt.Sprintf("Config file coreander.yml not found in %s/coreander", homeDir))
	}
	idx, err := bleve.Open(homeDir + "/coreander/coreander.db")
	if err == bleve.ErrorIndexPathDoesNotExist {
		log.Println("No index found, creating a new one")
		idx, err = indexer.Create(homeDir)
		if err != nil {
			log.Fatal(err)
		}
		err = indexer.Add(idx, cfg.LibraryPath)
		if err != nil {
			log.Fatal(err)
		}
	}
	webserver.Start(idx, cfg)
}
