package fsutil

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/afero"
)

// Evict deletes the least-recently-modified files in cacheDir until the total
// size is below maxSizeMB. No-op when maxSizeMB is 0.
func Evict(appFs afero.Fs, cacheDir string, maxSizeMB int) {
	if maxSizeMB <= 0 {
		return
	}
	maxBytes := int64(maxSizeMB) * 1024 * 1024

	type entry struct {
		path  string
		mtime int64
		size  int64
	}

	var entries []entry
	var total int64

	afero.Walk(appFs, cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		entries = append(entries, entry{path, info.ModTime().Unix(), info.Size()})
		total += info.Size()
		return nil
	})

	if total <= maxBytes {
		return
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].mtime < entries[j].mtime
	})

	for _, e := range entries {
		if total <= maxBytes {
			break
		}
		if err := appFs.Remove(e.path); err == nil {
			total -= e.size
		}
	}
}

// ReadFileBytes reads the raw bytes and file info for the given path.
func ReadFileBytes(appFs afero.Fs, path string) ([]byte, os.FileInfo, error) {
	f, err := appFs.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	data := make([]byte, info.Size())
	if _, err := f.Read(data); err != nil {
		return nil, nil, err
	}
	return data, info, nil
}

// WriteFileBytes creates any missing parent directories and writes data to path.
func WriteFileBytes(appFs afero.Fs, path string, data []byte) error {
	if err := appFs.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := appFs.Create(path)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	errc := f.Close()
	if err == nil {
		err = errc
	}
	return err
}
