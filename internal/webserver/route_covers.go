package webserver

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path/filepath"

	"github.com/disintegration/imaging"
	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/internal/metadata"
)

func routeCovers(c *fiber.Ctx, homeDir, libraryPath string, metadataReaders map[string]metadata.Reader, CoverMaxWidth int) error {
	var image []byte

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
			image, err = embedded.ReadFile("embedded/images/generic.jpg")
			if err != nil {
				log.Println(err)
				return fiber.ErrInternalServerError
			}
		} else {
			src, err := imaging.Open(fmt.Sprintf("%s/coreander/cache/covers/%s.jpg", homeDir, fileName))
			dst := imaging.Resize(src, CoverMaxWidth, 0, imaging.Box)
			if err != nil {
				log.Println(err)
				return fiber.ErrInternalServerError
			}

			buf := new(bytes.Buffer)
			imaging.Encode(buf, dst, imaging.JPEG)
			image = buf.Bytes()
			err = ioutil.WriteFile(fmt.Sprintf("%s/coreander/cache/covers/%s.jpg", homeDir, fileName), image, 0644)
			if err != nil {
				log.Println("Error creating", dst)
				return fiber.ErrInternalServerError
			}
		}
	} else if info.IsDir() {
		return fiber.ErrBadRequest
	} else {
		image, err = ioutil.ReadFile(fmt.Sprintf("%s/coreander/cache/covers/%s.jpg", homeDir, fileName))
		if err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
	}
	c.Response().Header.Set(fiber.HeaderContentType, "image/jpeg")
	c.Response().BodyWriter().Write(image)
	return nil
}
