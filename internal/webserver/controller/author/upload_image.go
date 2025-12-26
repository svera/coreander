package author

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
	"github.com/kovidgoyal/imaging"
	"github.com/spf13/afero"
	"github.com/valyala/fasthttp"
)

func (a *Controller) UploadImage(c *fiber.Ctx) error {
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
	img, err := imaging.Decode(fileReader)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid image file",
		})
	}

	// Resize if needed
	if a.config.AuthorImageMaxWidth > 0 && img.Bounds().Max.X >= a.config.AuthorImageMaxWidth {
		img = imaging.Resize(img, a.config.AuthorImageMaxWidth, 0, imaging.Box)
	}

	// Save image as JPEG (always save as .jpg regardless of input format)
	imageFileName := a.config.CacheDir + "/" + authorSlug + ".jpg"

	// Delete old file first to ensure modification time changes
	if exists, _ := afero.Exists(a.appFs, imageFileName); exists {
		if err := a.appFs.Remove(imageFileName); err != nil {
			log.Error(fmt.Errorf("error removing old author image '%s': %w", imageFileName, err))
		}
	}

	if err = a.saveImage(img, imageFileName); err != nil {
		log.Error(fmt.Errorf("error saving author image '%s': %w", imageFileName, err))
		return fiber.ErrInternalServerError
	}

	return nil
}
