package annotation

import (
	"log"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v5/internal/webserver/model"
)

func (a *Controller) Save(c fiber.Ctx) error {
	userID, slug, err := a.owner(c)
	if err != nil {
		return err
	}
	var body struct {
		CFI     string `json:"cfi"`
		Content string `json:"content"`
		Comment string `json:"comment"`
	}
	if err := c.Bind().Body(&body); err != nil {
		return fiber.ErrBadRequest
	}
	if !validCFI(body.CFI) || strings.TrimSpace(body.Content) == "" || len(body.Content) > 65536 {
		return fiber.NewError(fiber.StatusBadRequest, "A CFI (up to 8192 bytes) and annotated text (up to 65536 bytes) are required")
	}
	if len(body.Comment) > 65536 {
		return fiber.NewError(fiber.StatusBadRequest, "The comment must not exceed 65536 bytes")
	}
	if err := a.repository.Save(userID, slug, body.CFI, body.Content, body.Comment); err != nil {
		log.Printf("error saving text annotation: %v\n", err)
		return fiber.ErrInternalServerError
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (a *Controller) List(c fiber.Ctx) error {
	userID, slug, err := a.owner(c)
	if err != nil {
		return err
	}
	annotations, err := a.repository.List(userID, slug)
	if err != nil {
		log.Printf("error listing text annotations: %v\n", err)
		return fiber.ErrInternalServerError
	}
	return c.JSON(annotations)
}

func (a *Controller) Delete(c fiber.Ctx) error {
	userID, slug, err := a.owner(c)
	if err != nil {
		return err
	}
	var body struct {
		CFI string `json:"cfi"`
	}
	if err := c.Bind().Body(&body); err != nil {
		return fiber.ErrBadRequest
	}
	if !validCFI(body.CFI) {
		return fiber.NewError(fiber.StatusBadRequest, "A CFI (up to 8192 bytes) is required")
	}
	if err := a.repository.Delete(userID, slug, body.CFI); err != nil {
		log.Printf("error deleting text annotation: %v\n", err)
		return fiber.ErrInternalServerError
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func validCFI(cfi string) bool {
	return strings.HasPrefix(cfi, "epubcfi(") && strings.HasSuffix(cfi, ")") && len(cfi) <= 8192
}

func (a *Controller) owner(c fiber.Ctx) (int, string, error) {
	session, ok := c.Locals("Session").(model.Session)
	if !ok || session.ID == 0 {
		return 0, "", fiber.ErrForbidden
	}
	document, err := a.idx.Document(c.Params("slug"))
	if err != nil {
		log.Println(err)
		return 0, "", fiber.ErrInternalServerError
	}
	if document.Slug == "" {
		return 0, "", fiber.ErrNotFound
	}
	return int(session.ID), document.Slug, nil
}
