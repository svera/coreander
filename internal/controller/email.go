package controller

import (
	"fmt"
	"os"

	"github.com/gofiber/fiber/v2"
)

func Send(c *fiber.Ctx, libraryPath string, sender Sender, idx Reader) error {
	if c.FormValue("slug") == "" || c.FormValue("email") == "" {
		return fiber.ErrBadRequest
	}

	document, err := idx.Document(c.FormValue("slug"))
	if err != nil {
		return fiber.ErrBadRequest
	}

	if _, err := os.Stat(fmt.Sprintf("%s/%s", libraryPath, document.ID)); err != nil {
		return fiber.ErrBadRequest
	}

	go sender.SendDocument(c.FormValue("email"), libraryPath, document.ID)
	return nil
}
