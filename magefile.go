// +build mage

package main

import (
	"fmt"

	"github.com/magefile/mage/sh"
)

// Installs the application.
func Install() error {
	version, err := sh.Output("git", "describe", "--always", "--long", "--dirty")
	if err != nil {
		return err
	}
	return sh.Run("go", "install", "-ldflags", "-X main.version="+version)
}

// Creates an executable for the given platform. Possible platforms are "rpi32" and "osxintel".
func Build(platform string) error {
	envMap, err := env(platform)
	if err != nil {
		return err
	}
	version, err := sh.Output("git", "describe", "--always", "--long", "--dirty")
	if err != nil {
		return err
	}
	return sh.RunWith(envMap, "go", "build", "-ldflags", "-X main.version="+version)
}

func env(platform string) (map[string]string, error) {
	env := map[string]string{}

	switch platform {
	case "rpi32":
		return map[string]string{
			"GOOS":   "linux",
			"GOARCH": "arm",
			"GOARM":  "7",
		}, nil
	case "osxintel":
		return map[string]string{
			"GOOS":   "darwin",
			"GOARCH": "amd64",
		}, nil
	}

	return env, fmt.Errorf("Platform '%s' not supported", platform)
}
