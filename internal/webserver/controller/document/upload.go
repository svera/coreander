package document

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"slices"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
	"github.com/valyala/fasthttp"
)

func (d *Controller) UploadForm(c *fiber.Ctx) error {
	return c.Render("document/upload", fiber.Map{
		"Title":   "Upload document",
		"MaxSize": d.config.UploadDocumentMaxSize,
	}, "layout")
}

func (d *Controller) Upload(c *fiber.Ctx) error {
	templateVars := fiber.Map{
		"Title":   "Upload document",
		"MaxSize": d.config.UploadDocumentMaxSize,
	}

	file, err := c.FormFile("filename")
	if errors.Is(err, fasthttp.ErrMissingFile) {
		templateVars["Error"] = "Invalid file type"
		return c.Status(fiber.StatusBadRequest).Render("document/upload", templateVars, "layout")
	}

	allowedTypes := []string{"application/epub+zip", "application/pdf"}
	if !slices.Contains(allowedTypes, file.Header.Get("Content-Type")) {
		templateVars["Error"] = "Invalid file type"
		return c.Status(fiber.StatusBadRequest).Render("document/upload", templateVars, "layout")
	}

	if file.Size > int64(d.config.UploadDocumentMaxSize*1024*1024) {
		templateVars["Error"] = fmt.Sprintf("Document too large, the maximum allowed size is %d megabytes", d.config.UploadDocumentMaxSize)
		return c.Status(fiber.StatusRequestEntityTooLarge).Render("document/upload", templateVars, "layout")
	}

	destination := filepath.Join(d.config.LibraryPath, file.Filename)
	internalServerErrorStatus := c.Status(fiber.StatusInternalServerError).Render("document/upload", fiber.Map{
		"Title":   "Upload Document",
		"Error":   "Error uploading document",
		"MaxSize": d.config.UploadDocumentMaxSize,
	}, "layout")

	bytes, err := fileToBytes(file)
	if err != nil {
		log.Error(err)
		return internalServerErrorStatus
	}

	destFile, err := d.appFs.Create(destination)
	if err != nil {
		log.Error(err)
		return internalServerErrorStatus
	}

	defer destFile.Close()

	if _, err := destFile.Write(bytes); err != nil {
		log.Error(err)
		return internalServerErrorStatus
	}

	slug, err := d.idx.AddFile(destination)
	if err != nil {
		log.Error(err)
		d.appFs.Remove(destination)
		return internalServerErrorStatus
	}

	c.Cookie(&fiber.Cookie{
		Name:    "success",
		Value:   "true",
		Expires: time.Now().Add(24 * time.Hour),
	})
	return c.Redirect(fmt.Sprintf("/documents/%s", slug))
}

func fileToBytes(fileHeader *multipart.FileHeader) ([]byte, error) {
	f, err := fileHeader.Open()
	if err != nil {
		return []byte{}, err
	}
	defer f.Close()

	buf := bytes.NewBuffer(nil)
	if _, err := io.Copy(buf, f); err != nil {
		return []byte{}, err
	}

	return buf.Bytes(), nil
}
