package controller

import (
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v2/internal/metadata"
)

func Covers(c *fiber.Ctx, homeDir, libraryPath string, metadataReaders map[string]metadata.Reader, coverMaxWidth int, embedded embed.FS, idx Reader) error {
	c.Append("Cache-Time", "86400")

	var (
		image []byte
	)

	document, err := idx.Document(c.Params("ID"))
	if err != nil {
		return err
	}
	ext := filepath.Ext(document.Filename)
	if _, ok := metadataReaders[ext]; !ok {
		return fiber.ErrBadRequest
	}
	image, err = metadataReaders[ext].Cover(fmt.Sprintf("%s"+string(os.PathSeparator)+"%s", libraryPath, document.Filename), coverMaxWidth)
	if err != nil {
		log.Println(err)
		return fiber.ErrNotFound
	}

	c.Response().Header.Set(fiber.HeaderContentType, "image/jpeg")
	c.Response().BodyWriter().Write(image)
	return nil
}
