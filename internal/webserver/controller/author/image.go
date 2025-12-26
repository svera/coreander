package author

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kovidgoyal/imaging"
	"github.com/svera/coreander/v4/internal/datasource/wikidata"
)

func (a *Controller) Image(c *fiber.Ctx) error {
	authorSlug := strings.Split(c.Params("slug"), "_")[0]
	lang := c.Locals("Lang").(string)

	imageFileName := a.config.CacheDir + "/" + authorSlug + ".jpg"
	img, err := a.openImage(imageFileName)

	// Get file info for ETag and Last-Modified headers (only if file exists)
	var fileInfo os.FileInfo
	if err == nil {
		if info, statErr := a.appFs.Stat(imageFileName); statErr == nil {
			fileInfo = info
		}
	}

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
			// Use default image based on gender
			img, err = a.loadDefaultImage(author.Gender)
			if err != nil {
				log.Printf("author %s has no image and failed to load default: %v", authorSlug, err)
				return fiber.ErrNotFound
			}
			// Don't save default images to cache
		} else {
			img, err = a.readFromDataSource(author.DataSourceImage)
			if err != nil {
				log.Println(fmt.Errorf("error getting image from data source: %w", err))
				return fiber.ErrInternalServerError
			}

			if err = a.saveImage(img, imageFileName); err != nil {
				log.Println(fmt.Errorf("error saving image '%s' to cache: %w", imageFileName, err))
			}
			// Get file info after saving
			if info, statErr := a.appFs.Stat(imageFileName); statErr == nil {
				fileInfo = info
			}
		}
	}

	// Set cache headers based on whether file exists
	if shouldReturn304 := a.setupClientCache(c, fileInfo); shouldReturn304 {
		return c.Status(304).Send(nil)
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

// setupClientCache configures cache headers for the image response.
// Returns true if a 304 Not Modified response should be sent.
func (a *Controller) setupClientCache(c *fiber.Ctx, fileInfo os.FileInfo) bool {
	if fileInfo != nil {
		// If cache-busting query parameter is present, disable caching completely
		if c.Query("t") != "" {
			c.Set("Cache-Control", "no-cache, no-store, must-revalidate")
			c.Set("Pragma", "no-cache")
			c.Set("Expires", "0")
		} else {
			// Cached file exists - use ETag for cache validation
			etag := fmt.Sprintf(`"%x-%x"`, fileInfo.ModTime().Unix(), fileInfo.Size())
			c.Set("ETag", etag)
			c.Set("Last-Modified", fileInfo.ModTime().UTC().Format(http.TimeFormat))

			// Check If-None-Match header for 304 Not Modified
			if match := c.Get("If-None-Match"); match == etag {
				return true
			}

			// Set Cache-Control: allow caching but must revalidate when ETag changes
			cacheControl := fmt.Sprintf("public, max-age=%d, must-revalidate", a.config.ClientImageCacheTTL)
			c.Set("Cache-Control", cacheControl)
			c.Append("Cache-Time", fmt.Sprintf("%d", a.config.ServerImageCacheTTL))
		}
	} else {
		// For default images (not cached), allow short-term caching
		cacheControl := fmt.Sprintf("public, max-age=%d", a.config.ClientImageCacheTTL)
		c.Set("Cache-Control", cacheControl)
		c.Append("Cache-Time", fmt.Sprintf("%d", a.config.ServerImageCacheTTL))
	}
	return false
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

func (a *Controller) loadDefaultImage(gender float64) (image.Image, error) {
	var defaultImagePath string
	switch gender {
	case wikidata.GenderMale:
		defaultImagePath = "male.webp"
	case wikidata.GenderFemale:
		defaultImagePath = "female.webp"
	default:
		// Default to male for unknown gender
		defaultImagePath = "male.webp"
	}

	file, err := a.embeddedImagesFS.Open(defaultImagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open default image %s: %w", defaultImagePath, err)
	}
	defer file.Close()

	img, err := imaging.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("failed to decode default image %s: %w", defaultImagePath, err)
	}

	return img, nil
}
