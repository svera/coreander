package document

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/log"
	"github.com/valyala/fasthttp"
)

func (d *Controller) UploadForm(c fiber.Ctx) error {
	return c.Render("document/upload", fiber.Map{
		"Title":   "Upload document",
		"MaxSize": d.config.UploadDocumentMaxSize,
	}, "layout")
}

func (d *Controller) Upload(c fiber.Ctx) error {
	templateVars := fiber.Map{
		"Title":   "Upload document",
		"MaxSize": d.config.UploadDocumentMaxSize,
	}

	file, err := c.FormFile("filename")
	if errors.Is(err, fasthttp.ErrMissingFile) {
		templateVars["Error"] = "Invalid file type"
		return c.Status(fiber.StatusBadRequest).Render("document/upload", templateVars, "layout")
	}

	allowedExtensions := []string{".epub", ".pdf", ".cbz"}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !slices.Contains(allowedExtensions, ext) {
		templateVars["Error"] = "Invalid file type"
		return c.Status(fiber.StatusBadRequest).Render("document/upload", templateVars, "layout")
	}

	// Browsers often send application/zip or application/octet-stream for .cbz; accept by extension.
	allowedTypes := []string{"application/epub+zip", "application/pdf", "application/vnd.comicbook+zip", "application/x-cbz", "application/zip", "application/octet-stream", ""}
	contentType := strings.TrimSpace(file.Header.Get("Content-Type"))
	if !slices.Contains(allowedTypes, contentType) {
		templateVars["Error"] = "Invalid file type"
		return c.Status(fiber.StatusBadRequest).Render("document/upload", templateVars, "layout")
	}
	// application/zip only allowed for .cbz (reject generic zip uploaded as .epub/.pdf)
	if contentType == "application/zip" && ext != ".cbz" {
		templateVars["Error"] = "Invalid file type"
		return c.Status(fiber.StatusBadRequest).Render("document/upload", templateVars, "layout")
	}

	if file.Size > int64(d.config.UploadDocumentMaxSize*1024*1024) {
		templateVars["Error"] = fmt.Sprintf("Document too large, the maximum allowed size is %d megabytes", d.config.UploadDocumentMaxSize)
		return c.Status(fiber.StatusRequestEntityTooLarge).Render("document/upload", templateVars, "layout")
	}

	internalServerErrorStatus := c.Status(fiber.StatusInternalServerError).Render("document/upload", fiber.Map{
		"Title":   "Upload Document",
		"Error":   "Error uploading document",
		"MaxSize": d.config.UploadDocumentMaxSize,
	}, "layout")

	contents, err := fileToBytes(file)
	if err != nil {
		log.Error(err)
		return internalServerErrorStatus
	}

	slug, err := d.idx.NewFile(file.Filename, contents)
	if err != nil {
		log.Error(err)
		return internalServerErrorStatus
	}

	c.Cookie(&fiber.Cookie{
		Name:    "success-once",
		Value:   "Document uploaded successfully.",
		Expires: time.Now().Add(24 * time.Hour),
	})
	return c.Redirect().To(fmt.Sprintf("/documents/%s", slug))
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
