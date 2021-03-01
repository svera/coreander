// +build mage

package main

import (
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

// Creates an executable for the given platform. Possible platforms are "rpi32".
func Build(platform string) error {
	version, err := sh.Output("git", "describe", "--always", "--long", "--dirty")
	if err != nil {
		return err
	}
	return sh.RunWith(env(platform), "go", "build", "-ldflags", "-X main.version="+version)
}

func env(platform string) map[string]string {
	env := map[string]string{}

	switch platform {
	case "rpi32":
		env = map[string]string{
			"GOOS":   "linux",
			"GOARCH": "arm",
			"GOARM":  "7",
		}
	}

	return env
}
