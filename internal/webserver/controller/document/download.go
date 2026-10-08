package document

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/adaptor"
	"github.com/pgaskin/kepubify/v4/kepub"
	"github.com/svera/coreander/v5/internal/index"
)

func (d *Controller) Download(c fiber.Ctx) error {
	slug := c.Params("slug")

	conditional := c.Get("If-None-Match")
	if strings.EqualFold(c.Query("format"), "kepub") {
		conditional = ""
	}
	result, err := d.idx.FileForDownload(slug, conditional)
	if err != nil {
		log.Println(err)
		if errors.Is(err, index.ErrDocumentNotFound) {
			return fiber.ErrNotFound
		}
		return fiber.ErrInternalServerError
	}

	data := result.Data
	fileName := result.FileName
	contentType := result.ContentType
	etag := result.ETag

	if strings.ToLower(c.Query("format")) == "kepub" && result.ContentType == "application/epub+zip" {
		z, err := zip.NewReader(bytes.NewReader(result.Data), int64(len(result.Data)))
		if err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
		buf := bytes.NewBuffer(nil)
		if err := kepub.NewConverter().Convert(context.Background(), buf, z); err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
		data = buf.Bytes()
		etag = fmt.Sprintf(`"%x"`, sha256.Sum256(data))
		fileName = strings.TrimSuffix(filepath.Base(result.FileName), filepath.Ext(result.FileName)) + ".kepub.epub"
	}

	c.Set("ETag", etag)
	// Compression rewrites strong ETags, breaking validation of offline copies.
	c.Set("Cache-Control", "no-cache, no-transform")
	if c.Get("If-None-Match") == etag {
		return c.Status(http.StatusNotModified).Send(nil)
	}

	if contentType == "application/pdf" {
		return adaptor.HTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(fiber.HeaderContentType, contentType)
			w.Header().Set(fiber.HeaderContentDisposition, fmt.Sprintf("inline; filename=\"%s\"", fileName))
			http.ServeContent(w, r, fileName, time.Time{}, bytes.NewReader(data))
		})(c)
	}

	c.Response().Header.Set(fiber.HeaderContentType, contentType)
	c.Response().Header.Set(fiber.HeaderContentDisposition, fmt.Sprintf("inline; filename=\"%s\"", fileName))
	c.Response().BodyWriter().Write(data)
	return nil
}
