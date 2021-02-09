package metadata

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

type opf struct {
	Package  xml.Name `xml:"package"`
	Metadata struct {
		Title string `xml:"title"`
		Meta  []struct {
			Name    string `xml:"name,attr"`
			Content string `xml:"content,attr"`
		} `xml:"meta"`
	} `xml:"metadata"`
}

// EpubCover parses the book looking for a cover image, and extracts it to outputFolder
func EpubCover(bookFullPath string, outputFolder string) error {
	r, err := zip.OpenReader(bookFullPath)
	if err != nil {
		return err
	}
	defer r.Close()
	cover, err := getCoverFileName(r)
	if err != nil {
		return err
	}
	coverExt := filepath.Ext(cover)
	outputPath := fmt.Sprintf("%s/%s%s", outputFolder, filepath.Base(bookFullPath), coverExt)
	err = extractCover(r, cover, outputPath)
	if err != nil {
		return err
	}
	return nil
}

func getCoverFileName(r *zip.ReadCloser) (string, error) {
	for _, f := range r.File {
		if f.Name != "OEBPS/content.opf" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		cover, err := parseOPF(rc)
		if err != nil {
			return "", err
		}
		rc.Close()
		return cover, err
	}
	return "", fmt.Errorf("No content.opf file found")
}

func extractCover(r *zip.ReadCloser, coverFile, outputPath string) error {
	for _, f := range r.File {
		if f.Name != fmt.Sprintf("OEBPS/Images/%s", coverFile) {
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

func parseOPF(f io.ReadCloser) (string, error) {
	var unmarshalledOPF opf
	content, err := ioutil.ReadAll(f)
	if err != nil {
		return "", err
	}
	err = xml.Unmarshal(content, &unmarshalledOPF)
	if err != nil {
		return "", err
	}
	for _, val := range unmarshalledOPF.Metadata.Meta {
		if val.Name == "cover" {
			return val.Content, nil
		}
	}
	return "", fmt.Errorf("No cover found")
}
