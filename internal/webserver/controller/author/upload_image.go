package author

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/log"
	"github.com/kovidgoyal/imaging"
	"github.com/spf13/afero"
	"github.com/valyala/fasthttp"
)

func (a *Controller) UploadImage(c fiber.Ctx) error {
	authorSlug := c.Params("slug")
	if authorSlug == "" {
		return fiber.ErrBadRequest
	}

	file, err := c.FormFile("image")
	if errors.Is(err, fasthttp.ErrMissingFile) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "No image file provided",
		})
	}
	if err != nil {
		log.Error(err)
		return fiber.ErrInternalServerError
	}

	// Validate file type
	allowedTypes := []string{"image/jpeg", "image/jpg", "image/png"}
	contentType := file.Header.Get("Content-Type")
	if !slices.Contains(allowedTypes, contentType) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid file type. Only JPEG and PNG images are allowed.",
		})
	}

	// Validate file extension
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid file extension. Only .jpg, .jpeg, and .png files are allowed.",
		})
	}

	// Read file
	fileReader, err := file.Open()
	if err != nil {
		log.Error(err)
		return fiber.ErrInternalServerError
	}
	defer fileReader.Close()

	// Decode image directly from file reader
	img, err := imaging.Decode(fileReader, imaging.Backends(imaging.GO_IMAGE))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid image file",
		})
	}

	// Resize if needed
	if a.config.AuthorImageMaxWidth > 0 && img.Bounds().Max.X >= a.config.AuthorImageMaxWidth {
		img = imaging.Resize(img, a.config.AuthorImageMaxWidth, 0, imaging.Box)
	}

	jpgFileName := a.config.CacheDir + "/" + authorSlug + ".jpg"
	webpFileName := a.config.CacheDir + "/" + authorSlug + ".webp"

	// Delete old files first to ensure modification time changes
	for _, f := range []string{jpgFileName, webpFileName} {
		if exists, _ := afero.Exists(a.appFs, f); exists {
			if err := a.appFs.Remove(f); err != nil {
				log.Error(fmt.Errorf("error removing old author image '%s': %w", f, err))
			}
		}
	}

	if err = a.saveImage(img, jpgFileName); err != nil {
		log.Error(fmt.Errorf("error saving author image '%s': %w", jpgFileName, err))
		return fiber.ErrInternalServerError
	}

	if err = a.saveImageWebP(img, webpFileName); err != nil {
		log.Error(fmt.Errorf("error saving author webp image '%s': %w", webpFileName, err))
	}

	// Set cache-busting timestamp in response header
	// Get file info after saving to return the new modification time
	fileInfo, statErr := a.appFs.Stat(jpgFileName)
	if statErr == nil {
		c.Set("X-Image-Timestamp", fmt.Sprintf("%d", fileInfo.ModTime().Unix()))
	} else {
		// Fallback to current time if stat fails
		c.Set("X-Image-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	}

	return nil
}
