package author

import (
	"fmt"
	"log"
	"strings"

	"github.com/spf13/afero"
)

// MigrateJPEGsToWebP converts any cached .jpg author images to .webp in the authors/
// subdirectory and removes the originals.
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
		srcPath := a.config.CacheDir + "/" + name
		dstPath := a.config.CacheDir + "/authors/" + strings.TrimSuffix(name, ".jpg") + ".webp"

		if exists, _ := afero.Exists(a.appFs, dstPath); exists {
			a.appFs.Remove(srcPath)
			continue
		}
		img, err := a.openImage(srcPath)
		if err != nil {
			log.Println(fmt.Errorf("migrate: error opening %s: %w", srcPath, err))
			continue
		}
		if err := a.saveImage(img, dstPath); err != nil {
			log.Println(fmt.Errorf("migrate: error saving %s: %w", dstPath, err))
			continue
		}
		if err := a.appFs.Remove(srcPath); err != nil {
			log.Println(fmt.Errorf("migrate: error removing %s: %w", srcPath, err))
		}
	}
}
