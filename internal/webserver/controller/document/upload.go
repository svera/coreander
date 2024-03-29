package document

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"slices"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

func (d *Controller) UploadForm(c *fiber.Ctx) error {
	msg := ""
	if c.Query("success") != "" {
		msg = "Document uploaded successfully."
	}

	return c.Render("upload", fiber.Map{
		"Title":   "Coreander",
		"Message": msg,
		"MaxSize": d.config.UploadDocumentMaxSize,
	}, "layout")
}

func (d *Controller) Upload(c *fiber.Ctx) error {
	file, err := c.FormFile("filename")
	if err != nil {
		if errors.Is(err, fasthttp.ErrMissingFile) {
			return c.Status(fiber.StatusBadRequest).Render("upload", fiber.Map{
				"Title": "Coreander",
				"Error": "Invalid file type",
			}, "layout")
		}
	}

	allowedTypes := []string{"application/epub+zip", "application/pdf"}
	if !slices.Contains(allowedTypes, file.Header.Get("Content-Type")) {
		return c.Status(fiber.StatusBadRequest).Render("upload", fiber.Map{
			"Title": "Coreander",
			"Error": "Invalid file type",
		}, "layout")
	}

	if file.Size > int64(d.config.UploadDocumentMaxSize*1024*1024) {
		return c.Status(fiber.StatusRequestEntityTooLarge).Render("upload", fiber.Map{
			"Title": "Coreander",
			"Error": fmt.Sprintf("Document too large, the maximum allowed size is %d megabytes", d.config.UploadDocumentMaxSize),
		}, "layout")
	}

	destination := filepath.Join(d.config.LibraryPath, file.Filename)
	internalServerErrorStatus := c.Status(fiber.StatusInternalServerError).Render("upload", fiber.Map{
		"Title": "Coreander",
		"Error": "Error uploading document",
	}, "layout")

	bytes, err := fileToBytes(file)
	if err != nil {
		return internalServerErrorStatus
	}

	destFile, err := d.appFs.Create(destination)
	if err != nil {
		return internalServerErrorStatus
	}

	if _, err := destFile.Write(bytes); err != nil {
		return internalServerErrorStatus
	}

	destFile.Close()
	if err := d.idx.AddFile(destination); err != nil {
		os.Remove(destination)
		return internalServerErrorStatus
	}

	return c.Redirect(fmt.Sprintf("/%s/upload?success=1", c.Params("lang")))
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
