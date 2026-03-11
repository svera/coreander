//go:build !linux

package index

// StartFileWatcher is a no-op on non-Linux platforms.
func (b *BleveIndexer) StartFileWatcher() {
}
