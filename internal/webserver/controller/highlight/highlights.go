package highlight

import (
	"log"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v3/internal/index"
	"github.com/svera/coreander/v3/internal/result"
	"github.com/svera/coreander/v3/internal/webserver/infrastructure"
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

	var session model.User
	if val, ok := c.Locals("Session").(model.User); ok {
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

	highlights, err := h.hlRepository.Highlights(int(user.ID), page, model.ResultsPerPage)
	if err != nil {
		return fiber.ErrInternalServerError
	}

	docs, err := h.idx.Documents(highlights.Hits())
	if err != nil {
		return fiber.ErrInternalServerError
	}

	docsSortedByHighlightedDate := make([]index.Document, len(docs))

	i := 0
	for path := range docs {
		docsSortedByHighlightedDate[i] = docs[path]
		docsSortedByHighlightedDate[i].Highlighted = true
		i++
	}

	paginatedResults := result.NewPaginated[[]index.Document](
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
		"WordsPerMinute":         h.wordsPerMinute,
	}, "layout")
}
