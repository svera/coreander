//go:build !linux
// +build !linux

package main

import (
	"github.com/svera/coreander/internal/index"
)

func fileWatcher(idx *index.BleveIndexer, libPath string) {
}
