package controller

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/internal/jwtclaimsreader"
	"github.com/svera/coreander/internal/model"
)

type IdxWriter interface {
	RemoveFile(file string) error
}

func Delete(c *fiber.Ctx, libraryPath string, writer IdxWriter, appFs afero.Fs) error {
	session := jwtclaimsreader.SessionData(c)

	if session.Role != model.RoleAdmin {
		return fiber.ErrForbidden
	}

	if c.FormValue("file") == "" {
		return fiber.ErrBadRequest
	}

	if strings.Contains(c.FormValue("file"), ".."+string(os.PathSeparator)) {
		return fiber.ErrBadRequest
	}

	fullPath := fmt.Sprintf("%s"+string(os.PathSeparator)+"%s", libraryPath, c.FormValue("file"))

	if _, err := appFs.Stat(fullPath); err != nil {
		return fiber.ErrBadRequest
	}

	if err := writer.RemoveFile(fullPath); err != nil {
		return fiber.ErrInternalServerError
	}

	if err := appFs.Remove(fullPath); err != nil {
		log.Printf("error removing file %s", fullPath)
	}

	return nil
}
