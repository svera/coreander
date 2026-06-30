package document

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/deepteams/webp"
	"github.com/gofiber/fiber/v3"
	"github.com/kovidgoyal/imaging"
)

func (d *Controller) Cover(c fiber.Ctx) error {
	slug := c.Params("slug")
	coversDir := d.config.CacheDir + "/covers"
	webpPath := coversDir + "/" + slug + ".webp"

	// Serve from cache if available
	if data, fileInfo, ok := d.readCoverCache(webpPath); ok {
		return d.serveCover(c, data, fileInfo)
	}

	// Cache miss — extract from source
	jpegBytes, err := d.idx.Cover(slug, d.config.CoverMaxWidth)
	if err != nil {
		log.Println(err)
		return fiber.ErrNotFound
	}

	img, err := imaging.Decode(bytes.NewReader(jpegBytes), imaging.Backends(imaging.GO_IMAGE))
	if err != nil {
		log.Println(fmt.Errorf("cover: decode error for %s: %w", slug, err))
		return fiber.ErrInternalServerError
	}

	buf := new(bytes.Buffer)
	if err = webp.Encode(buf, img, &webp.EncoderOptions{Lossless: false, Quality: 80}); err != nil {
		log.Println(fmt.Errorf("cover: webp encode error for %s: %w", slug, err))
		return fiber.ErrInternalServerError
	}
	data := buf.Bytes()

	// Save to cache (best-effort)
	var fileInfo os.FileInfo
	if err := d.saveCoverCache(coversDir, webpPath, data); err != nil {
		log.Println(fmt.Errorf("cover: cache save error for %s: %w", slug, err))
	} else {
		fileInfo, _ = d.appFs.Stat(webpPath)
	}

	return d.serveCover(c, data, fileInfo)
}

func (d *Controller) readCoverCache(path string) ([]byte, os.FileInfo, bool) {
	f, err := d.appFs.Open(path)
	if err != nil {
		return nil, nil, false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, nil, false
	}
	data := make([]byte, info.Size())
	if _, err := f.Read(data); err != nil {
		return nil, nil, false
	}
	return data, info, true
}

func (d *Controller) saveCoverCache(dir, path string, data []byte) error {
	if err := d.appFs.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := d.appFs.Create(path)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	errc := f.Close()
	if err == nil {
		err = errc
	}
	return err
}

func (d *Controller) serveCover(c fiber.Ctx, data []byte, fileInfo os.FileInfo) error {
	c.Append("Cache-Time", fmt.Sprintf("%d", d.config.ServerImageCacheTTL))

	if fileInfo != nil {
		etag := fmt.Sprintf(`"%x-%x"`, fileInfo.ModTime().Unix(), fileInfo.Size())
		c.Set("ETag", etag)
		c.Set("Last-Modified", fileInfo.ModTime().UTC().Format(http.TimeFormat))
		if c.Get("If-None-Match") == etag {
			return c.Status(304).Send(nil)
		}
		c.Set("Cache-Control", fmt.Sprintf("public, max-age=%d, must-revalidate", d.config.ClientImageCacheTTL))
	} else {
		c.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", d.config.ClientImageCacheTTL))
	}

	c.Response().Header.Set(fiber.HeaderContentType, "image/webp")
	c.Response().BodyWriter().Write(data)
	return nil
}
