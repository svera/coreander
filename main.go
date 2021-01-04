package main

import (
	"fmt"
	"log"
	"os"

	"github.com/ilyakaznacheev/cleanenv"
	"github.com/qinains/fastergoding"
	"github.com/svera/coreander/config"
	"github.com/svera/coreander/indexer"
	"github.com/svera/coreander/webserver"
)

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
	idx, err := indexer.New(homeDir, cfg.LibraryPath)
	webserver.Start(idx, cfg)
}
