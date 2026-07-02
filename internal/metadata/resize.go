package metadata

import (
	"image"

	"github.com/kovidgoyal/imaging"
)

func resize(src image.Image, coverMaxWidth int) image.Image {
	if coverMaxWidth > 0 {
		return imaging.Resize(src, coverMaxWidth, 0, imaging.Box)
	}
	return src
}
