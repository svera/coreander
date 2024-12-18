//go:build mage
// +build mage

package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"

	"github.com/magefile/mage/sh"
)

// Installs the application.
func Install() error {
	version, err := version()
	if err != nil {
		return err
	}
	return sh.Run("go", "install", "-ldflags", "-X main.version="+version)
}

// Creates an executable for the given platform.
func Build(platform string) error {
	envMap, err := env(platform)
	if err != nil {
		return err
	}
	version, err := version()
	if err != nil {
		return err
	}
	return buildEnv(platform, version, envMap)
}

// Build binary files of the current version for all supported platforms and zip them
func Release() error {
	platforms := []string{"linux32", "linux64", "linuxarm32", "linuxarm64", "osxintel", "osxapple", "win64"}
	version, err := version()
	if err != nil {
		return err
	}
	for _, platform := range platforms {
		fmt.Printf("Building binary for %s\n", platform)
		envMap, err := env(platform)
		if err != nil {
			return err
		}
		err = buildEnv(platform, version, envMap)
		if err != nil {
			return err
		}
		err = createZip("coreander", "coreander_"+platform+".zip")
		if err != nil {
			return err
		}
	}
	return nil
}

func buildEnv(platform, version string, envMap map[string]string) error {
	return sh.RunWith(envMap, "go", "build", "-ldflags", "-X main.version="+version)
}

func env(platform string) (map[string]string, error) {
	env := map[string]string{}

	switch platform {
	case "linux32":
		return map[string]string{
			"GOOS":   "linux",
			"GOARCH": "386",
		}, nil
	case "linux64":
		return map[string]string{
			"GOOS":   "linux",
			"GOARCH": "amd64",
		}, nil
	case "linuxarm32":
		return map[string]string{
			"GOOS":   "linux",
			"GOARCH": "arm",
			"GOARM":  "7",
		}, nil
	case "linuxarm64":
		return map[string]string{
			"GOOS":   "linux",
			"GOARCH": "arm64",
		}, nil
	case "osxintel":
		return map[string]string{
			"GOOS":   "darwin",
			"GOARCH": "amd64",
		}, nil
	case "osxapple":
		return map[string]string{
			"GOOS":   "darwin",
			"GOARCH": "arm64",
		}, nil
	case "win32":
		return map[string]string{
			"GOOS":   "windows",
			"GOARCH": "386",
		}, nil
	case "win64":
		return map[string]string{
			"GOOS":   "windows",
			"GOARCH": "amd64",
		}, nil
	}

	return env, fmt.Errorf("Platform '%s' not supported", platform)
}

func createZip(fileName, zipFileName string) error {
	zipFile, err := os.Create(zipFileName)
	if err != nil {
		return err
	}
	defer zipFile.Close()
	w := zip.NewWriter(zipFile)
	defer w.Close()

	if err = addFileToZip(w, fileName); err != nil {
		return err
	}
	return nil
}

func addFileToZip(zipWriter *zip.Writer, filename string) error {
	fileToZip, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer fileToZip.Close()

	// Get the file information
	info, err := fileToZip.Stat()
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}

	// Using FileInfoHeader() above only uses the basename of the file. If we want
	// to preserve the folder structure we can overwrite this with the full path.
	header.Name = filename

	// Change to deflate to gain better compression
	// see http://golang.org/pkg/archive/zip/#pkg-constants
	header.Method = zip.Deflate

	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, fileToZip)
	return err
}

func version() (string, error) {
	return sh.Output("git", "describe", "--always", "--dirty", "--tags")
}
