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
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v3/internal/webserver/controller"
	"github.com/svera/coreander/v3/internal/webserver/model"
	"github.com/valyala/fasthttp"
)

func (d *Controller) UploadForm(c *fiber.Ctx) error {
	var session model.User
	if val, ok := c.Locals("Session").(model.User); ok {
		session = val
	}

	if session.Role != model.RoleAdmin {
		return fiber.ErrForbidden
	}

	upload := fmt.Sprintf(
		"%s://%s%s/%s/upload",
		c.Protocol(),
		d.config.Hostname,
		controller.UrlPort(c.Protocol(), d.config.Port),
		c.Params("lang"),
	)

	msg := ""
	if ref := string(c.Request().Header.Referer()); strings.HasPrefix(ref, upload) {
		msg = "Document uploaded successfully."
	}

	return c.Render("upload", fiber.Map{
		"Title":   "Coreander",
		"Message": msg,
		"MaxSize": d.config.UploadDocumentMaxSize,
	}, "layout")
}

func (d *Controller) Upload(c *fiber.Ctx) error {
	session := c.Locals("Session").(model.User)

	if session.Role != model.RoleAdmin {
		return fiber.ErrForbidden
	}

	file, err := c.FormFile("filename")
	if err != nil {
		if errors.Is(err, fasthttp.ErrMissingFile) {
			return fiber.ErrBadRequest
		}
		return err
	}

	allowedTypes := []string{"application/epub+zip", "application/pdf"}
	if !slices.Contains(allowedTypes, file.Header.Get("Content-Type")) {
		return c.Status(fiber.StatusBadRequest).Render("upload", fiber.Map{
			"Title": "Coreander",
			"Error": "Invalid file type",
		}, "layout")
	}

	if file.Size > int64(d.config.UploadDocumentMaxSize*1024*1024) {
		errorMessage := fmt.Sprintf("Document too large, the maximum allowed size is %d megabytes", d.config.UploadDocumentMaxSize)
		return c.Status(fiber.StatusRequestEntityTooLarge).Render("upload", fiber.Map{
			"Title": "Coreander",
			"Error": errorMessage,
		}, "layout")
	}

	errorMessage := ""
	destination := filepath.Join(d.config.LibraryPath, file.Filename)

	bytes, err := fileToBytes(file)
	if err != nil {
		errorMessage = "Error uploading document"
	}
	destFile, err := d.appFs.Create(destination)
	if err != nil {
		errorMessage = "Error uploading document"
	}
	destFile.Write(bytes)

	if err := d.idx.AddFile(destination); err != nil {
		os.Remove(destination)
		errorMessage = "Error indexing document"
	}

	if errorMessage != "" {
		return c.Status(fiber.StatusInternalServerError).Render("upload", fiber.Map{
			"Title": "Coreander",
			"Error": errorMessage,
		}, "layout")

	}

	return c.Redirect(fmt.Sprintf("/%s/upload", c.Params("lang")))
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
