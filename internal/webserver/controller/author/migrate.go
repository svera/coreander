package author

import (
	"fmt"
	"log"
	"strings"

	"github.com/spf13/afero"
)

// MigrateJPEGsToWebP converts any cached .jpg author images to .webp and removes the originals.
// Intended to be called once as a goroutine at startup.
// Remove on the next major version bump, after all users have had a chance to migrate.
func (a *Controller) MigrateJPEGsToWebP() {
	entries, err := afero.ReadDir(a.appFs, a.config.CacheDir)
	if err != nil {
		log.Println(fmt.Errorf("migrate: error reading cache dir: %w", err))
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".jpg") {
			continue
		}
		jpgPath := a.config.CacheDir + "/" + name
		webpPath := a.config.CacheDir + "/" + strings.TrimSuffix(name, ".jpg") + ".webp"

		if exists, _ := afero.Exists(a.appFs, webpPath); exists {
			continue
		}

		img, err := a.openImage(jpgPath)
		if err != nil {
			log.Println(fmt.Errorf("migrate: error opening %s: %w", jpgPath, err))
			continue
		}
		if err := a.saveImageWebP(img, webpPath); err != nil {
			log.Println(fmt.Errorf("migrate: error saving %s: %w", webpPath, err))
			continue
		}
		if err := a.appFs.Remove(jpgPath); err != nil {
			log.Println(fmt.Errorf("migrate: error removing %s: %w", jpgPath, err))
		}
	}
}
