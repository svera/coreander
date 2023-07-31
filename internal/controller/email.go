package controller

import (
	"fmt"
	"net/mail"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func Send(c *fiber.Ctx, libraryPath string, sender Sender, idx IdxReader) error {
	if strings.Trim(c.FormValue("slug"), " ") == "" {
		return fiber.ErrBadRequest
	}

	if _, err := mail.ParseAddress(c.FormValue("email")); err != nil {
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
