package author

import (
	"bytes"
	"fmt"
	"image"
	"log"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/kovidgoyal/imaging"
)

func (a *Controller) Portrait(c *fiber.Ctx) error {
	imageFileName := c.Params("slug") + "." + c.Params("extension")
	authorSlug := c.Params("slug")
	lang := c.Locals("Lang").(string)

	img, err := imaging.Open(a.config.CacheDir + "/" + imageFileName)
	if err != nil {
		author, err := a.idx.Author(authorSlug, lang)
		if err != nil {
			log.Println(fmt.Errorf("error getting author from index: %w", err))
			return err
		}
		img, err = a.getImage(author.Image)
		if err != nil {
			log.Println(fmt.Errorf("error getting image from source: %w", err))
			return err
		}
		if err = imaging.Save(img, a.config.CacheDir+"/"+imageFileName); err != nil {
			log.Println(fmt.Errorf("error saving image '%s' to cache: %w", a.config.CacheDir+"/"+imageFileName, err))
		}
	}
	buf := new(bytes.Buffer)
	if err = imaging.Encode(buf, img, imaging.JPEG); err != nil {
		return err
	}
	c.Response().Header.Set(fiber.HeaderContentType, "image/jpeg")
	c.Response().BodyWriter().Write(buf.Bytes())
	return nil
}

func (a *Controller) getImage(path string) (image.Image, error) {
	res, err := http.Get(path)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()
	img, err := imaging.Decode(res.Body)
	if err != nil {
		return nil, err
	}
	if img.Bounds().Max.X >= 600 {
		img = imaging.Resize(img, 600, 0, imaging.Box)
	}
	return img, nil
}
