package webserver

import (
	"github.com/gofiber/fiber/v2"
)

type Sender interface {
	SendDocument(address string, libraryPath string, fileName string) error
}

func routeSend(c *fiber.Ctx, libraryPath string, fileName string, address string, sender Sender) {
	go sender.SendDocument(address, libraryPath, fileName)
}
