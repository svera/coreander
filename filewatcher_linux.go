package main

import (
	"log"

	"github.com/rjeczalik/notify"
	"github.com/svera/coreander/internal/index"
	"github.com/svera/coreander/internal/metadata"
)

func fileWatcher(idx *index.BleveIndexer, libPath string, readers map[string]metadata.Reader) {
	log.Printf("Starting file watcher on %s\n", cfg.LibPath)
	c := make(chan notify.EventInfo, 1)
	if err := notify.Watch(cfg.LibPath, c, notify.InCloseWrite, notify.InMovedTo, notify.InMovedFrom, notify.InDelete); err != nil {
		log.Fatal(err)
	}

	defer notify.Stop(c)

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
