package highlight

import (
	"fmt"
	"log"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/result"
	"github.com/svera/coreander/v4/internal/webserver/infrastructure"
	"github.com/svera/coreander/v4/internal/webserver/model"
	"github.com/svera/coreander/v4/internal/webserver/view"
)

func (h *Controller) List(c *fiber.Ctx) error {
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

	user, err := h.usrRepository.FindByUsername(session.Username)
	if err != nil {
		log.Println(err.Error())
		return fiber.ErrInternalServerError
	}

	if user == nil {
		return fiber.ErrNotFound
	}

	emailSendingConfigured := true
	if _, ok := h.sender.(*infrastructure.NoEmail); ok {
		emailSendingConfigured = false
	}

	highlights, totalHits, err := h.sortedHighlights(c, page, user)
	if err != nil {
		return err
	}

	if c.Query("view") == "latest" {
		return h.latest(c, highlights, emailSendingConfigured)
	}

	paginatedResults := result.NewPaginated[[]index.Document](
		model.ResultsPerPage,
		page,
		totalHits,
		highlights,
	)

	url := fmt.Sprintf("/highlights?view=list&page=%d", page)

	layout := "layout"
	if c.Query("view") == "list" {
		layout = ""
	}

	err = c.Render("highlights", fiber.Map{
		"Results":                paginatedResults,
		"Paginator":              view.Pagination(model.MaxPagesNavigator, paginatedResults, nil),
		"Title":                  "Highlights",
		"EmailSendingConfigured": emailSendingConfigured,
		"EmailFrom":              h.sender.From(),
		"WordsPerMinute":         h.wordsPerMinute,
		"Url":                    url,
	}, layout)

	if err != nil {
		log.Println(err)
	}

	return nil
}

func (h *Controller) sortedHighlights(c *fiber.Ctx, page int, user *model.User) ([]index.Document, int, error) {
	highlightsAmount, err := strconv.Atoi(c.Query("amount", strconv.Itoa(model.ResultsPerPage)))
	if err != nil {
		return nil, 0, fiber.ErrBadRequest
	}
	if highlightsAmount < 1 {
		highlightsAmount = model.ResultsPerPage
	}

	docsSortedByHighlightedDate, err := h.hlRepository.Highlights(int(user.ID), page, highlightsAmount)
	if err != nil {
		log.Println(err)
		return nil, 0, fiber.ErrInternalServerError
	}

	if docsSortedByHighlightedDate.TotalPages() < page {
		page = docsSortedByHighlightedDate.TotalPages()
		docsSortedByHighlightedDate, err = h.hlRepository.Highlights(int(user.ID), page, highlightsAmount)
		if err != nil {
			log.Println(err)
			return nil, 0, fiber.ErrInternalServerError
		}
	}

	docs, err := h.idx.Documents(docsSortedByHighlightedDate.Hits())
	if err != nil {
		log.Println(err)
		return nil, 0, fiber.ErrInternalServerError
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

	return highlights, docsSortedByHighlightedDate.TotalHits(), nil
}

func (h *Controller) latest(c *fiber.Ctx, highlights []index.Document, emailSendingConfigured bool) error {
	err := c.Render("partials/latest-highlights", fiber.Map{
		"Highlights":             highlights,
		"EmailSendingConfigured": emailSendingConfigured,
		"EmailFrom":              h.sender.From(),
		"WordsPerMinute":         h.wordsPerMinute,
	})
	if err != nil {
		log.Println(err)
	}

	return nil
}
