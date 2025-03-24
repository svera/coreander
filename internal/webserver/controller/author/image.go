package author

import (
	"bytes"
	"fmt"
	"image"
	"log"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/kovidgoyal/imaging"
)

func (a *Controller) Image(c *fiber.Ctx) error {
	authorSlug := strings.Split(c.Params("slug"), "_")[0]
	lang := c.Locals("Lang").(string)

	imageFileName := a.config.CacheDir + "/" + authorSlug + ".jpg"
	img, err := a.openImage(imageFileName)
	if err != nil {
		author, err := a.idx.Author(authorSlug, lang)
		if author.Name == "" {
			return fiber.ErrNotFound
		}
		if err != nil {
			log.Println(fmt.Errorf("error getting author from index: %w", err))
			return fiber.ErrInternalServerError
		}

		img, err = a.readFromDataSource(author.DataSourceImage)
		if err != nil {
			log.Println(fmt.Errorf("error getting image from data source: %w", err))
			return fiber.ErrInternalServerError
		}

		if err = a.saveImage(img, imageFileName); err != nil {
			log.Println(fmt.Errorf("error saving image '%s' to cache: %w", imageFileName, err))
		}
	}
	buf := new(bytes.Buffer)
	if err = imaging.Encode(buf, img, imaging.JPEG); err != nil {
		log.Println(fmt.Errorf("error encoding image to JPEG: %w", err))
		return fiber.ErrInternalServerError
	}
	c.Response().Header.Set(fiber.HeaderContentType, "image/jpeg")
	c.Response().BodyWriter().Write(buf.Bytes())
	return nil
}

func (a *Controller) readFromDataSource(path string) (image.Image, error) {
	res, err := http.Get(path)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()
	img, err := imaging.Decode(res.Body)
	if err != nil {
		return nil, err
	}
	if a.config.AuthorImageMaxWidth > 0 && img.Bounds().Max.X >= a.config.AuthorImageMaxWidth {
		img = imaging.Resize(img, a.config.AuthorImageMaxWidth, 0, imaging.Box)
	}
	return img, nil
}

func (a *Controller) openImage(filename string, opts ...imaging.DecodeOption) (image.Image, error) {
	file, err := a.appFs.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return imaging.Decode(file, opts...)
}

func (a *Controller) saveImage(img image.Image, filename string, opts ...imaging.EncodeOption) (err error) {
	f, err := imaging.FormatFromFilename(filename)
	if err != nil {
		return err
	}
	file, err := a.appFs.Create(filename)
	if err != nil {
		return err
	}
	err = imaging.Encode(file, img, f, opts...)
	errc := file.Close()
	if err == nil {
		err = errc
	}
	return err
}
