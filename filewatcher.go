//go:build !linux

package main

import (
	"github.com/svera/coreander/v2/internal/index"
)

func fileWatcher(idx *index.BleveIndexer, libPath string) {
}
