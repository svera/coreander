package highlight

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v3/internal/infrastructure"
	"github.com/svera/coreander/v3/internal/jwtclaimsreader"
	"github.com/svera/coreander/v3/internal/model"
	"github.com/svera/coreander/v3/internal/webserver/controller"
)

func (h *Controller) Highlights(c *fiber.Ctx) error {
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
		"Paginator":              controller.Pagination(model.MaxPagesNavigator, highlights.TotalPages, page, nil),
		"Title":                  "Highlights",
		"EmailSendingConfigured": emailSendingConfigured,
		"EmailFrom":              h.sender.From(),
		"Session":                session,
		"WordsPerMinute":         h.wordsPerMinute,
	}, "layout")
}
