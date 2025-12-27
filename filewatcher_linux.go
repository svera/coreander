package main

import (
	"log"
	"path/filepath"
	"strings"

	"github.com/rjeczalik/notify"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

func fileWatcher(idx *index.BleveIndexer, libPath string, hlRepo *model.HighlightRepository, readingRepo *model.ReadingRepository) {
	log.Printf("Starting file watcher on %s\n", libPath)
	c := make(chan notify.EventInfo, 1)
	if err := notify.Watch(libPath, c, notify.InCloseWrite, notify.InMovedTo, notify.InMovedFrom, notify.InDelete); err != nil {
		log.Fatal(err)
	}

	defer notify.Stop(c)

	for {
		select {
		case ei := <-c:
			if ei.Event() == notify.InCloseWrite || ei.Event() == notify.InMovedFrom {
				if _, err := idx.AddFile(ei.Path()); err != nil {
					log.Printf("Error indexing new file: %s\n", ei.Path())
				}
			}
			if ei.Event() == notify.InDelete || ei.Event() == notify.InMovedTo {
				if err := idx.RemoveFile(ei.Path()); err != nil {
					log.Printf("Error removing file from index: %s\n", ei.Path())
				}
				// Normalize path: remove library path prefix, same as RemoveFile does
				documentPath := strings.Replace(ei.Path(), libPath, "", 1)
				documentPath = strings.TrimPrefix(documentPath, string(filepath.Separator))
				// Remove from highlights table
				if err := hlRepo.RemoveDocument(documentPath); err != nil {
					log.Printf("Error removing file %s from highlights table: %s\n", documentPath, err)
				}
				// Remove from reading table
				if err := readingRepo.RemoveDocument(documentPath); err != nil {
					log.Printf("Error removing file %s from reading table: %s\n", documentPath, err)
				}
			}
		}
	}
}
