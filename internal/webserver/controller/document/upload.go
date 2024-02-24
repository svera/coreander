package document

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gofiber/fiber/v2"
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

	resetPassword := fmt.Sprintf(
		"%s://%s%s/%s/upload",
		c.Protocol(),
		d.config.Hostname,
		d.urlPort(c),
		c.Params("lang"),
	)

	msg := ""
	if ref := string(c.Request().Header.Referer()); strings.HasPrefix(ref, resetPassword) {
		msg = "Document uploaded successfully."
	}

	return c.Render("upload", fiber.Map{
		"Title":   "Coreander",
		"Message": msg,
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

	errorMessage := ""
	destination := filepath.Join(d.config.LibraryPath, file.Filename)
	if err := c.SaveFile(file, destination); err != nil {
		errorMessage = "Error uploading document"
	}

	if err := d.idx.AddFile(destination); err != nil {
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
