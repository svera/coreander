package controller

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v3/internal/jwtclaimsreader"
	"github.com/svera/coreander/v3/internal/metadata"
	"github.com/svera/coreander/v3/internal/model"
)

type IdxWriter interface {
	Document(ID string) (metadata.Metadata, error)
	RemoveFile(file string) error
}

func Delete(c *fiber.Ctx, libraryPath string, writer IdxWriter, appFs afero.Fs) error {
	session := jwtclaimsreader.SessionData(c)

	if session.Role != model.RoleAdmin {
		return fiber.ErrForbidden
	}

	if c.FormValue("slug") == "" {
		return fiber.ErrBadRequest
	}

	document, err := writer.Document(c.FormValue("slug"))
	if err != nil {
		fmt.Println(err)
		return fiber.ErrBadRequest
	}

	fullPath := filepath.Join(libraryPath, document.ID)
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
