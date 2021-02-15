package main

import (
	"fmt"
	"log"
	"time"

	"github.com/blevesearch/bleve/v2"

	"github.com/rjeczalik/notify"
	"github.com/spf13/afero"
	"github.com/svera/coreander/internal/index"
	"github.com/svera/coreander/internal/metadata"
	"github.com/svera/coreander/internal/webserver"
)

func run(cfg Config, homeDir string, metadataReaders map[string]metadata.Reader, appFs afero.Fs) {
	var idx *index.BleveIndexer
	var err error
	indexFile, err := bleve.Open(homeDir + "/coreander/db")
	if err == nil {
		idx = index.NewBleve(indexFile, cfg.LibPath, metadataReaders)
	}
	if err == bleve.ErrorIndexPathDoesNotExist {
		log.Println("No index found, creating a new one")
		indexMapping := bleve.NewIndexMapping()
		index.AddMappings(indexMapping)
		indexFile, err := bleve.New(homeDir+"/coreander/db", indexMapping)
		if err != nil {
			log.Fatal(err)
		}
		idx = index.NewBleve(indexFile, cfg.LibPath, metadataReaders)
	}
	c := make(chan notify.EventInfo, 1)
	if err := notify.Watch(cfg.LibPath, c, notify.InCloseWrite, notify.InMovedTo, notify.InMovedFrom, notify.InDelete); err != nil {
		log.Fatal(err)
	}
	defer func() {
		notify.Stop(c)
		idx.Close()
	}()

	go func() {
		start := time.Now().Unix()
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
	app := webserver.New(idx, cfg.LibPath, homeDir)
	err = app.Listen(fmt.Sprintf(":%s", cfg.Port))
	if err != nil {
		log.Fatal(err)
	}
}

func fileWatcher(c <-chan (notify.EventInfo), idx *index.BleveIndexer, libPath string, readers map[string]metadata.Reader) {
	for {
		select {
		case ei := <-c:
			if ei.Event() == notify.InCloseWrite || ei.Event() == notify.InMovedFrom {
				if err := idx.AddFile(ei.Path()); err != nil {
					log.Printf("Error indexing new file: %s\n", ei.Path())
				}
			}
			if ei.Event() == notify.InDelete || ei.Event() == notify.InMovedTo {
				if err := idx.RemoveFile(ei.Path()); err != nil {
					log.Printf("Error removing file from index: %s\n", ei.Path())
				}
			}
		}
	}
}
