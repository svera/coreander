package author

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kovidgoyal/imaging"
)

func (a *Controller) Image(c *fiber.Ctx) error {
	// Set cache control headers
	cacheControl := fmt.Sprintf("public, max-age=%d", a.config.ClientImageCacheTTL)
	c.Set("Cache-Control", cacheControl)
	c.Append("Cache-Time", fmt.Sprintf("%d", a.config.ServerImageCacheTTL))

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

		// Check if author has an image
		if author.DataSourceImage == "" {
			log.Printf("author %s has no image", authorSlug)
			return fiber.ErrNotFound
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
	if path == "" {
		return nil, fmt.Errorf("image path is empty")
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "GET", path, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; coreander/1.0; +https://github.com/svera/coreander)")
	if err != nil {
		return nil, fmt.Errorf("failed to create request for %s: %w", path, err)
	}

	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch image from %s: %w", path, err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch image from %s: HTTP %d", path, res.StatusCode)
	}

	img, err := imaging.Decode(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image from %s: %w", path, err)
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
