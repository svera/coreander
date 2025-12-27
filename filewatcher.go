//go:build !linux

package main

import (
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

func fileWatcher(idx *index.BleveIndexer, libPath string, hlRepo *model.HighlightRepository, readingRepo *model.ReadingRepository) {
}
