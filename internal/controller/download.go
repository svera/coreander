package controller

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func Download(c *fiber.Ctx, homeDir, libraryPath string, idx Reader) error {
	c.Append("Cache-Time", "86400")

	document, err := idx.Document(c.Params("slug"))
	if err != nil {
		return fiber.ErrBadRequest
	}

	fullPath := fmt.Sprintf("%s"+string(os.PathSeparator)+"%s", libraryPath, document.ID)

	if _, err := os.Stat(fullPath); err != nil {
		return fiber.ErrNotFound
	}

	ext := strings.ToLower(filepath.Ext(document.ID))

	if err != nil {
		log.Println(err)
		return fiber.ErrNotFound
	}

	if ext == ".epub" {
		c.Response().Header.Set(fiber.HeaderContentType, "application/epub+zip")
	} else {
		c.Response().Header.Set(fiber.HeaderContentType, "application/pdf")
	}

	file, err := os.Open(fullPath)
	if err != nil {
		return fiber.ErrInternalServerError
	}
	contents, err := io.ReadAll(file)
	if err != nil {
		return fiber.ErrInternalServerError
	}

	c.Response().BodyWriter().Write(contents)
	return nil
}
