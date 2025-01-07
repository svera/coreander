package metadata

import (
	"bytes"
	"image"

	"github.com/gofiber/fiber/v2"
	"github.com/kovidgoyal/imaging"
)

func resize(src image.Image, coverMaxWidth int, err error) ([]byte, error) {
	dst := imaging.Resize(src, coverMaxWidth, 0, imaging.Box)
	if err != nil {
		return nil, fiber.ErrInternalServerError
	}

	buf := new(bytes.Buffer)
	imaging.Encode(buf, dst, imaging.JPEG)
	return buf.Bytes(), nil
}
