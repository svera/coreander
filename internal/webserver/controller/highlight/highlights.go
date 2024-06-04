package highlight

import (
	"log"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/result"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"github.com/svera/coreander/v4/internal/webserver/model"
	"github.com/svera/coreander/v4/internal/webserver/view"
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

	var session model.Session
	if val, ok := c.Locals("Session").(model.Session); ok {
		session = val
	}

	if session.WordsPerMinute > 0 {
		h.wordsPerMinute = session.WordsPerMinute
	}

	user, err := h.usrRepository.FindByUsername(c.Params("username"))
	if err != nil {
		log.Println(err.Error())
		return fiber.ErrInternalServerError
	}

	if user == nil {
		return fiber.ErrNotFound
	}

	docsSortedByHighlightedDate, err := h.hlRepository.Highlights(int(user.ID), page, model.ResultsPerPage)
	if err != nil {
		return fiber.ErrInternalServerError
	}

	docs, err := h.idx.Documents(docsSortedByHighlightedDate.Hits())
	if err != nil {
		return fiber.ErrInternalServerError
	}

	highlights := make([]index.Document, 0, len(docs))
	for _, path := range docsSortedByHighlightedDate.Hits() {
		if _, ok := docs[path]; !ok {
			continue
		}
		doc := docs[path]
		doc.Highlighted = true
		highlights = append(highlights, doc)
	}

	paginatedResults := result.NewPaginated[[]index.Document](
		model.ResultsPerPage,
		page,
		docsSortedByHighlightedDate.TotalHits(),
		highlights,
	)

	return c.Render("highlights", fiber.Map{
		"Results":                paginatedResults,
		"Paginator":              view.Pagination(model.MaxPagesNavigator, paginatedResults, nil),
		"Title":                  "Highlights",
		"EmailSendingConfigured": emailSendingConfigured,
		"EmailFrom":              h.sender.From(),
		"WordsPerMinute":         h.wordsPerMinute,
	}, "layout")
}
