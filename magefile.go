//go:build mage

package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/magefile/mage/sh"
)

// Installs the application.
func Install() error {
	version, err := version()
	if err != nil {
		return err
	}
	return sh.Run("go", "install", "-ldflags", "-s -w -X main.version="+version)
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
		err = createZip(binaryName(platform), "coreander_"+platform+".zip")
		if err != nil {
			return err
		}
	}
	return nil
}

func buildEnv(platform, version string, envMap map[string]string) error {
	return sh.RunWith(envMap, "go", "build", "-o", binaryName(platform), "-ldflags", "-s -w -X main.version="+version)
}

func binaryName(platform string) string {
	if strings.HasPrefix(platform, "win") {
		return "coreander.exe"
	}
	return "coreander"
}

func env(platform string) (map[string]string, error) {
	env := map[string]string{"CGO_ENABLED": "0"}

	switch platform {
	case "linux32":
		env["GOOS"], env["GOARCH"] = "linux", "386"
	case "linux64":
		env["GOOS"], env["GOARCH"] = "linux", "amd64"
	case "linuxarm32":
		env["GOOS"], env["GOARCH"], env["GOARM"] = "linux", "arm", "7"
	case "linuxarm64":
		env["GOOS"], env["GOARCH"] = "linux", "arm64"
	case "osxintel":
		env["GOOS"], env["GOARCH"] = "darwin", "amd64"
	case "osxapple":
		env["GOOS"], env["GOARCH"] = "darwin", "arm64"
	case "win32":
		env["GOOS"], env["GOARCH"] = "windows", "386"
	case "win64":
		env["GOOS"], env["GOARCH"] = "windows", "amd64"
	default:
		return nil, fmt.Errorf("Platform '%s' not supported", platform)
	}

	if env["GOARCH"] == "amd64" {
		env["GOAMD64"] = "v1"
	}
	return env, nil
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
