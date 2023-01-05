package webserver

import (
	"fmt"
	"log"
	"net/url"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/internal/metadata"
)

func routeCovers(c *fiber.Ctx, homeDir, libraryPath string, metadataReaders map[string]metadata.Reader, coverMaxWidth int) error {
	var (
		image []byte
	)

	fileName, err := url.QueryUnescape(c.Params("filename"))
	if err != nil {
		return err
	}
	ext := filepath.Ext(fileName)
	if _, ok := metadataReaders[ext]; !ok {
		return fiber.ErrBadRequest
	}
	image, err = metadataReaders[ext].Cover(fmt.Sprintf("%s/%s", libraryPath, fileName), coverMaxWidth)
	if err != nil {
		log.Println(err)
		image, err = embedded.ReadFile("embedded/images/generic.jpg")
		if err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
	}

	c.Response().Header.Set(fiber.HeaderContentType, "image/jpeg")
	c.Response().BodyWriter().Write(image)
	return nil
}
