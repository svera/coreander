package controller

import (
	"log"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v3/internal/metadata"
)

func Cover(c *fiber.Ctx, homeDir, libraryPath string, metadataReaders map[string]metadata.Reader, coverMaxWidth int, idx IdxReader) error {
	c.Append("Cache-Time", "86400")

	var (
		image []byte
	)

	document, err := idx.Document(c.Params("slug"))
	if err != nil {
		return fiber.ErrBadRequest
	}
	ext := filepath.Ext(document.ID)

	if _, ok := metadataReaders[ext]; !ok {
		return fiber.ErrBadRequest
	}
	image, err = metadataReaders[ext].Cover(filepath.Join(libraryPath, document.ID), coverMaxWidth)
	if err != nil {
		log.Println(err)
		return fiber.ErrNotFound
	}

	c.Response().Header.Set(fiber.HeaderContentType, "image/jpeg")
	c.Response().BodyWriter().Write(image)
	return nil
}
