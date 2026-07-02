package document

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/deepteams/webp"
	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v5/internal/webserver/controller/fsutil"
)

func (d *Controller) Cover(c fiber.Ctx) error {
	slug := c.Params("slug")
	webpPath := d.config.CacheDir + "/covers/" + slug + ".webp"

	// Serve from cache if available
	if data, fileInfo, err := fsutil.ReadFileBytes(d.appFs, webpPath); err == nil {
		return d.serveCover(c, data, fileInfo)
	}

	// Cache miss — extract from source
	img, err := d.idx.Cover(slug, d.config.CoverMaxWidth)
	if err != nil {
		log.Println(err)
		return fiber.ErrNotFound
	}

	buf := new(bytes.Buffer)
	if err = webp.Encode(buf, img, &webp.EncoderOptions{Lossless: false, Quality: 80}); err != nil {
		log.Println(fmt.Errorf("cover: webp encode error for %s: %w", slug, err))
		return fiber.ErrInternalServerError
	}
	data := buf.Bytes()

	var fileInfo os.FileInfo
	if err := fsutil.WriteFileBytes(d.appFs, webpPath, data); err != nil {
		log.Println(fmt.Errorf("cover: cache save error for %s: %w", slug, err))
	} else {
		fileInfo, _ = d.appFs.Stat(webpPath)
		go fsutil.Evict(d.appFs, d.config.CacheDir, d.config.CacheMaxSize)
	}

	return d.serveCover(c, data, fileInfo)
}

func (d *Controller) serveCover(c fiber.Ctx, data []byte, fileInfo os.FileInfo) error {
	c.Append("Cache-Time", fmt.Sprintf("%d", d.config.ServerImageCacheTTL))

	if fileInfo != nil {
		etag := fmt.Sprintf(`"%x-%x"`, fileInfo.ModTime().Unix(), fileInfo.Size())
		c.Set("ETag", etag)
		c.Set("Last-Modified", fileInfo.ModTime().UTC().Format(http.TimeFormat))
		if c.Get("If-None-Match") == etag {
			return c.Status(http.StatusNotModified).Send(nil)
		}
		c.Set("Cache-Control", fmt.Sprintf("public, max-age=%d, must-revalidate", d.config.ClientImageCacheTTL))
	} else {
		c.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", d.config.ClientImageCacheTTL))
	}

	c.Response().Header.Set(fiber.HeaderContentType, "image/webp")
	c.Response().BodyWriter().Write(data)
	return nil
}
