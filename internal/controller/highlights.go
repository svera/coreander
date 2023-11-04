package controller

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/infrastructure"
	"github.com/svera/coreander/v4/internal/jwtclaimsreader"
	"github.com/svera/coreander/v4/internal/model"
	"github.com/svera/coreander/v4/internal/search"
)

type highlightsRepository interface {
	Highlights(userID int, page int, resultsPerPage int) (search.PaginatedResult, error)
	Highlight(userID int, documentPath string) error
	Remove(userID int, documentPath string) error
}

type Highlights struct {
	hlRepository   highlightsRepository
	usrRepository  usersRepository
	idx            IdxReader
	sender         Sender
	wordsPerMinute float64
}

func NewHighlights(hlRepository highlightsRepository, usrRepository usersRepository, sender Sender, wordsPerMinute float64, idx IdxReader) *Highlights {
	return &Highlights{
		hlRepository:   hlRepository,
		usrRepository:  usrRepository,
		idx:            idx,
		sender:         sender,
		wordsPerMinute: wordsPerMinute,
	}
}

func (h *Highlights) Highlights(c *fiber.Ctx) error {
	emailSendingConfigured := true
	if _, ok := h.sender.(*infrastructure.NoEmail); ok {
		emailSendingConfigured = false
	}

	page, err := strconv.Atoi(c.Query("page"))
	if err != nil {
		page = 1
	}

	session := jwtclaimsreader.SessionData(c)
	if session.WordsPerMinute > 0 {
		h.wordsPerMinute = session.WordsPerMinute
	}

	user, err := h.usrRepository.FindByUuid(c.Params("uuid"))
	if err != nil {
		return fiber.ErrInternalServerError
	}

	if user == nil {
		return fiber.ErrNotFound
	}

	highlights, err := h.hlRepository.Highlights(int(user.ID), page, model.ResultsPerPage)
	if err != nil {
		return fiber.ErrInternalServerError
	}
	for i, highlight := range highlights.Hits {
		docs, err := h.idx.Documents([]string{highlight.ID})
		if err != nil {
			return fiber.ErrInternalServerError
		}
		highlights.Hits[i] = docs[0]
		highlights.Hits[i].Highlighted = true
	}

	return c.Render("highlights", fiber.Map{
		"Results":                highlights.Hits,
		"Total":                  highlights.TotalHits,
		"Paginator":              pagination(model.MaxPagesNavigator, highlights.TotalPages, page, nil),
		"Title":                  "Highlights",
		"EmailSendingConfigured": emailSendingConfigured,
		"EmailFrom":              h.sender.From(),
		"Session":                session,
		"WordsPerMinute":         h.wordsPerMinute,
	}, "layout")
}

func (h *Highlights) Highlight(c *fiber.Ctx) error {
	session := jwtclaimsreader.SessionData(c)

	user, err := h.usrRepository.FindByUuid(session.Uuid)
	if err != nil {
		return fiber.ErrBadRequest
	}

	document, err := h.idx.Document(c.FormValue("slug"))
	if err != nil {
		return fiber.ErrBadRequest
	}

	return h.hlRepository.Highlight(int(user.ID), document.ID)
}

func (h *Highlights) Remove(c *fiber.Ctx) error {
	session := jwtclaimsreader.SessionData(c)

	user, err := h.usrRepository.FindByUuid(session.Uuid)
	if err != nil {
		return fiber.ErrBadRequest
	}

	document, err := h.idx.Document(c.FormValue("slug"))
	if err != nil {
		return fiber.ErrBadRequest
	}

	return h.hlRepository.Remove(int(user.ID), document.ID)
}
