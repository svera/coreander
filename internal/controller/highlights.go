package controller

import (
	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v3/internal/jwtclaimsreader"
	"github.com/svera/coreander/v3/internal/model"
)

type highlightsRepository interface {
	Highlights(userID int, page int, resultsPerPage int) ([]model.Highlight, error)
	Highlight(userID int, documentPath string) error
	Remove(userID int, documentPath string) error
}

type Highlights struct {
	repository highlightsRepository
	idx        IdxReader
}

func NewHighlights(repository highlightsRepository, idx IdxReader) *Highlights {
	return &Highlights{
		repository: repository,
		idx:        idx,
	}
}

func (h *Highlights) Highlight(c *fiber.Ctx) error {
	session := jwtclaimsreader.SessionData(c)

	document, err := h.idx.Document(c.FormValue("slug"))
	if err != nil {
		return fiber.ErrBadRequest
	}

	/*
		fullPath := filepath.Join(libraryPath, document.ID)
		if _, err := appFs.Stat(fullPath); err != nil {
			return fiber.ErrBadRequest
		}*/

	return h.repository.Highlight(int(session.ID), document.ID)
}

func (h *Highlights) Remove(c *fiber.Ctx) error {
	session := jwtclaimsreader.SessionData(c)

	document, err := h.idx.Document(c.Params("slug"))
	if err != nil {
		return fiber.ErrBadRequest
	}

	/*
		fullPath := filepath.Join(libraryPath, document.ID)
		if _, err := appFs.Stat(fullPath); err != nil {
			return fiber.ErrBadRequest
		}*/

	return h.repository.Remove(int(session.ID), document.ID)
}
