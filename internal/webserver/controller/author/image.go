package author

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/deepteams/webp"
	"github.com/gofiber/fiber/v3"
	"github.com/kovidgoyal/imaging"
	"github.com/svera/coreander/v5/internal/datasource/wikidata"
	"github.com/svera/coreander/v5/internal/webserver/controller/fsutil"
)

func (a *Controller) Image(c fiber.Ctx) error {
	authorSlug := strings.Split(c.Params("slug"), "_")[0]
	imageFileName := a.config.CacheDir + "/authors/" + authorSlug + ".webp"

	// Cache hit: serve raw bytes, no re-encoding
	if fileInfo, err := a.appFs.Stat(imageFileName); err == nil {
		if a.setupClientCache(c, fileInfo) {
			return c.Status(http.StatusNotModified).Send(nil)
		}
		if data, _, err := fsutil.ReadFileBytes(a.appFs, imageFileName); err == nil {
			c.Response().Header.Set(fiber.HeaderContentType, "image/webp")
			c.Response().BodyWriter().Write(data)
			return nil
		}
	}

	// Cache miss: fetch, encode, save, serve
	lang := c.Locals("Lang").(string)
	author, authorErr := a.idx.Author(authorSlug, lang)
	if author.Name == "" {
		return fiber.ErrNotFound
	}
	if authorErr != nil {
		log.Println(fmt.Errorf("error getting author from index: %w", authorErr))
		return fiber.ErrInternalServerError
	}

	if author.DataSourceImage == "" {
		data, err := a.loadDefaultImage(author.Gender)
		if err != nil {
			log.Printf("author %s has no image and failed to load default: %v", authorSlug, err)
			return fiber.ErrNotFound
		}
		a.setupClientCache(c, nil)
		c.Response().Header.Set(fiber.HeaderContentType, "image/webp")
		c.Response().BodyWriter().Write(data)
		return nil
	}

	img, err := a.readFromDataSource(author.DataSourceImage)
	if err != nil {
		log.Println(fmt.Errorf("error getting image from data source: %w", err))
		return fiber.ErrInternalServerError
	}

	buf := new(bytes.Buffer)
	if err = webp.Encode(buf, img, &webp.EncoderOptions{Lossless: false, Quality: 80}); err != nil {
		log.Println(fmt.Errorf("error encoding image to WebP: %w", err))
		return fiber.ErrInternalServerError
	}
	data := buf.Bytes()

	var fileInfo os.FileInfo
	if saveErr := a.saveImage(img, imageFileName); saveErr != nil {
		log.Println(fmt.Errorf("error saving webp image '%s' to cache: %w", imageFileName, saveErr))
	} else {
		fileInfo, _ = a.appFs.Stat(imageFileName)
		go fsutil.Evict(a.appFs, a.config.CacheDir, a.config.CacheMaxSize)
	}

	if a.setupClientCache(c, fileInfo) {
		return c.Status(http.StatusNotModified).Send(nil)
	}
	c.Response().Header.Set(fiber.HeaderContentType, "image/webp")
	c.Response().BodyWriter().Write(data)
	return nil
}

func (a *Controller) setupClientCache(c fiber.Ctx, fileInfo os.FileInfo) bool {
	if c.Query("t") != "" {
		c.Set("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Set("Pragma", "no-cache")
		c.Set("Expires", "0")
		return false
	}

	c.Append("Cache-Time", fmt.Sprintf("%d", a.config.ServerImageCacheTTL))

	if fileInfo != nil {
		etag := fmt.Sprintf(`"%x-%x"`, fileInfo.ModTime().Unix(), fileInfo.Size())
		c.Set("ETag", etag)
		c.Set("Last-Modified", fileInfo.ModTime().UTC().Format(http.TimeFormat))
		if c.Get("If-None-Match") == etag {
			return true
		}
		c.Set("Cache-Control", fmt.Sprintf("public, max-age=%d, must-revalidate", a.config.ClientImageCacheTTL))
	} else {
		c.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", a.config.ClientImageCacheTTL))
	}

	return false
}

func (a *Controller) readFromDataSource(path string) (image.Image, error) {
	if path == "" {
		return nil, fmt.Errorf("image path is empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

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
	decodeOpts := append([]imaging.DecodeOption{imaging.Backends(imaging.GO_IMAGE)}, opts...)
	return imaging.Decode(file, decodeOpts...)
}

func (a *Controller) saveImage(img image.Image, filename string) error {
	if err := a.appFs.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		return err
	}
	file, err := a.appFs.Create(filename)
	if err != nil {
		return err
	}
	err = webp.Encode(file, img, &webp.EncoderOptions{Lossless: false, Quality: 80})
	errc := file.Close()
	if err == nil {
		err = errc
	}
	return err
}

func (a *Controller) loadDefaultImage(gender float64) ([]byte, error) {
	var defaultImagePath string
	switch gender {
	case wikidata.GenderMale:
		defaultImagePath = "male.webp"
	case wikidata.GenderFemale:
		defaultImagePath = "female.webp"
	default:
		defaultImagePath = "male.webp"
	}

	file, err := a.embeddedImagesFS.Open(defaultImagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open default image %s: %w", defaultImagePath, err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read default image %s: %w", defaultImagePath, err)
	}

	return data, nil
}
