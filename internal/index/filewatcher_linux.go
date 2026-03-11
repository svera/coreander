//go:build linux

package index

import (
	"log"

	"github.com/rjeczalik/notify"
)

// StartFileWatcher starts watching the library path for file changes and updates the index.
// It blocks until the process exits. Call it in a goroutine.
func (b *BleveIndexer) StartFileWatcher() {
	log.Printf("Starting file watcher on %s\n", b.libraryPath)
	c := make(chan notify.EventInfo, 1)
	if err := notify.Watch(b.libraryPath, c, notify.InCloseWrite, notify.InMovedTo, notify.InMovedFrom, notify.InDelete); err != nil {
		log.Fatal(err)
	}

	defer notify.Stop(c)

	for ei := range c {
		if ei.Event() == notify.InCloseWrite || ei.Event() == notify.InMovedFrom {
			if _, err := b.indexFile(ei.Path()); err != nil {
				log.Printf("Error indexing new file: %s\n", ei.Path())
			}
		}
		if ei.Event() == notify.InDelete || ei.Event() == notify.InMovedTo {
			if err := b.removeFile(ei.Path()); err != nil {
				log.Printf("Error removing file from index: %s\n", ei.Path())
			}
		}
	}
}
