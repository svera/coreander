package metadata

import (
	"archive/zip"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"path/filepath"
	"sort"
	"strings"
)

// OpenZipEntry opens a file inside a zip by name and returns a ReadCloser.
func OpenZipEntry(r *zip.ReadCloser, name string) (io.ReadCloser, error) {
	for _, f := range r.File {
		if f.Name != name {
			continue
		}
		return f.Open()
	}
	return nil, fmt.Errorf("zip entry %q not found", name)
}

// ImageMegapixelsFromZip reads a zip entry as an image and returns its size in megapixels (width*height/1e6).
func ImageMegapixelsFromZip(r *zip.ReadCloser, name string) (float64, error) {
	rc, err := OpenZipEntry(r, name)
	if err != nil || rc == nil {
		return 0, err
	}
	defer rc.Close()
	cfg, _, err := image.DecodeConfig(rc)
	if err != nil {
		return 0, err
	}
	return float64(cfg.Width*cfg.Height) / 1e6, nil
}

// ImageExtensions is the set of file extensions treated as images (for listing images in zip-based comics).
var ImageExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
	".webp": true, ".bmp": true, ".tiff": true, ".tif": true,
}

// SortedImageEntriesFromZip returns the names of non-directory zip entries whose extension is in ImageExtensions, sorted by name.
func SortedImageEntriesFromZip(r *zip.ReadCloser) []string {
	var names []string
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(f.Name))
		if ImageExtensions[ext] {
			names = append(names, f.Name)
		}
	}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	return names
}
