package controller

import (
	"github.com/gofiber/fiber/v2"
)

type Sender interface {
	SendDocument(address string, libraryPath string, fileName string) error
}

func Send(c *fiber.Ctx, libraryPath string, fileName string, address string, sender Sender) {
	go sender.SendDocument(address, libraryPath, fileName)
}
