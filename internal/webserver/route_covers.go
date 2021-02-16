package webserver

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/internal/metadata"
)

func coversRoute(c *fiber.Ctx, libraryPath, homeDir string, metadataReaders map[string]metadata.Reader) error {
	fileName, err := url.QueryUnescape(c.Params("filename"))
	if err != nil {
		return err
	}
	ext := filepath.Ext(fileName)
	if _, ok := metadataReaders[ext]; !ok {
		return fiber.ErrBadRequest
	}
	info, err := os.Stat(fmt.Sprintf("%s/coreander/cache/covers/%s.jpg", homeDir, fileName))
	if os.IsNotExist(err) {
		err = metadataReaders[ext].Cover(
			fmt.Sprintf("%s/%s", libraryPath, fileName),
			fmt.Sprintf("%s/coreander/cache/covers", homeDir),
		)
		if err != nil {
			log.Println(err)
			dir, _ := os.Getwd()
			input, err := ioutil.ReadFile(dir + "/public/images/generic.jpg")
			if err != nil {
				log.Println(err)
				return fiber.ErrInternalServerError
			}

			destinationFile := fmt.Sprintf("%s/coreander/cache/covers/%s.jpg", homeDir, fileName)
			err = ioutil.WriteFile(destinationFile, input, 0644)
			if err != nil {
				log.Println("Error creating", destinationFile)
				return fiber.ErrInternalServerError
			}
		}
	} else if info.IsDir() {
		return fiber.ErrBadRequest
	}
	image, err := ioutil.ReadFile(fmt.Sprintf("%s/coreander/cache/covers/%s.jpg", homeDir, fileName))
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	c.Response().Header.Set(fiber.HeaderContentType, "image/jpeg")
	c.Response().BodyWriter().Write(image)
	return nil
}
