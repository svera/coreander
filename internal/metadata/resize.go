package metadata

import (
	"bytes"
	"image"

	"github.com/disintegration/imaging"
	"github.com/gofiber/fiber/v2"
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
