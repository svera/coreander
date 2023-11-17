package highlight

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v3/internal/metadata"
	"github.com/svera/coreander/v3/internal/result"
	"github.com/svera/coreander/v3/internal/webserver/infrastructure"
	"github.com/svera/coreander/v3/internal/webserver/jwtclaimsreader"
	"github.com/svera/coreander/v3/internal/webserver/model"
	"github.com/svera/coreander/v3/internal/webserver/view"
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

	docs, err := h.idx.Documents(highlights.Hits())
	if err != nil {
		return fiber.ErrInternalServerError
	}

	docsSortedByHighlightedDate := make([]metadata.Document, len(docs))
	for i, path := range highlights.Hits() {
		docsSortedByHighlightedDate[i] = docs[path]
		docsSortedByHighlightedDate[i].Highlighted = true
	}

	paginatedResults := result.NewPaginated[[]metadata.Document](
		model.ResultsPerPage,
		page,
		highlights.TotalHits(),
		docsSortedByHighlightedDate,
	)

	return c.Render("highlights", fiber.Map{
		"Results":                paginatedResults,
		"Paginator":              view.Pagination(model.MaxPagesNavigator, paginatedResults, nil),
		"Title":                  "Highlights",
		"EmailSendingConfigured": emailSendingConfigured,
		"EmailFrom":              h.sender.From(),
		"Session":                session,
		"WordsPerMinute":         h.wordsPerMinute,
	}, "layout")
}
