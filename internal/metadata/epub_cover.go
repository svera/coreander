package metadata

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Cover parses the book looking for a cover image, and extracts it to outputFolder
func (e EpubReader) Cover(bookFullPath string, outputFolder string) error {
	reader := EpubReader{}
	meta, err := reader.Metadata(bookFullPath)
	if err != nil {
		return err
	}
	if meta.Cover == "" {
		return fmt.Errorf("No cover image set in %s", bookFullPath)
	}

	coverExt := filepath.Ext(meta.Cover)
	outputPath := fmt.Sprintf("%s/%s%s", outputFolder, filepath.Base(bookFullPath), coverExt)
	r, err := zip.OpenReader(bookFullPath)
	if err != nil {
		return err
	}
	defer r.Close()
	err = extractCover(r, meta.Cover, outputPath)
	if err != nil {
		return err
	}
	return nil
}

func extractCover(r *zip.ReadCloser, coverFile, outputPath string) error {
	for _, f := range r.File {
		if f.Name != fmt.Sprintf("OEBPS/%s", coverFile) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		outFile, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		_, err = io.Copy(outFile, rc)
		if err != nil {
			return err
		}
		outFile.Close()
		rc.Close()
		return nil
	}
	return fmt.Errorf("No cover image found")
}
