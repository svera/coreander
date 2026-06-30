package fsutil

import (
	"os"

	"github.com/spf13/afero"
)

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
