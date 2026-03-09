//go:build !386 && !arm

package webserver

import (
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/compress"
)

func addCompressMiddleware(app *fiber.App) {
	app.Use(compress.New())
}
