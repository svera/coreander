package controller

import (
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
)

type Sender interface {
	SendDocument(address string, libraryPath string, fileName string) error
}

func Send(c *fiber.Ctx, libraryPath string, fileName string, address string, sender Sender) error {
	if c.FormValue("file") == "" || c.FormValue("email") == "" {
		return fiber.ErrBadRequest
	}
	if strings.Contains(c.FormValue("file"), string(os.PathSeparator)) {
		return fiber.ErrBadRequest
	}

	go sender.SendDocument(address, libraryPath, fileName)
	return nil
}
