package fsutil

import (
	"os"
	"path/filepath"

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
