package controller

import (
	"fmt"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func Send(c *fiber.Ctx, libraryPath string, fileName string, address string, sender Sender) error {
	if c.FormValue("file") == "" || c.FormValue("email") == "" {
		return fiber.ErrBadRequest
	}

	if strings.Contains(c.FormValue("file"), string(os.PathSeparator)) {
		return fiber.ErrBadRequest
	}

	if _, err := os.Stat(fmt.Sprintf("%s/%s", libraryPath, c.FormValue("file"))); err != nil {
		return fiber.ErrBadRequest
	}

	go sender.SendDocument(address, libraryPath, fileName)
	return nil
}
