// +build !linux

package main

import (
	"fmt"
	"log"
	"os"

	"github.com/ilyakaznacheev/cleanenv"

	"github.com/spf13/afero"
	"github.com/svera/coreander/internal/metadata"
)

func main() {
	//fastergoding.Run() // hot reload
	var cfg Config
	var appFs = afero.NewOsFs()

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
	if err = os.MkdirAll(fmt.Sprintf("%s/coreander/cache/covers", homeDir), os.ModePerm); err != nil {
		log.Fatal(fmt.Errorf("Couldn't create %s, exiting", fmt.Sprintf("%s/coreander/cache/covers", homeDir)))
	}

	metadataReaders := map[string]metadata.Reader{
		".epub": metadata.Epub,
	}

	run(cfg, homeDir, metadataReaders, appFs)
}
