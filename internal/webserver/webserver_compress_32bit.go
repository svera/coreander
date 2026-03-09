//go:build 386 || arm

package webserver

import "github.com/gofiber/fiber/v3"

// addCompressMiddleware is a no-op on 32-bit (386, arm) to avoid pulling in the compress
// middleware, which depends on the etag package (math.MaxUint32 overflows int on 32-bit).
func addCompressMiddleware(app *fiber.App) {}
