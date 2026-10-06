package webserver

import (
	"encoding/json"
	"io/fs"
	"path"
	"strings"

	"github.com/gofiber/fiber/v3"
)

func readerOfflineWorker(assetVersion string) (string, error) {
	assets := []string{"/css/reader.css"}
	err := fs.WalkDir(jsFS, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || name == "reader-service-worker.js" {
			return nil
		}
		include := false
		switch path.Dir(name) {
		case ".":
			include = strings.HasPrefix(name, "reader") || name == "asset-version.js" || name == "menu.js"
		case "foliate-js", "foliate-js/ui":
			include = strings.HasSuffix(name, ".js")
		default:
			include = strings.HasPrefix(name, "foliate-js/vendor/")
		}
		if include {
			assets = append(assets, "/js/"+name)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	config, err := json.Marshal(struct {
		Assets       []string `json:"assets"`
		AssetVersion string   `json:"assetVersion"`
	}{assets, assetVersion})
	if err != nil {
		return "", err
	}
	source, err := fs.ReadFile(jsFS, "reader-service-worker.js")
	if err != nil {
		return "", err
	}
	return strings.Replace(string(source), "__READER_CONFIG__", string(config), 1), nil
}

func serveReaderOfflineWorker(source string) fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Set("Content-Type", "text/javascript; charset=utf-8")
		c.Set("Cache-Control", "no-cache")
		return c.SendString(source)
	}
}
