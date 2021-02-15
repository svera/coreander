// +build !linux

package main

import (
	"github.com/svera/coreander/internal/index"
	"github.com/svera/coreander/internal/metadata"
)

func fileWatcher(idx *index.BleveIndexer, libPath string, readers map[string]metadata.Reader) {
	return
}
