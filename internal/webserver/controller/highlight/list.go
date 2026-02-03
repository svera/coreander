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

func (h *Controller) List(c *fiber.Ctx) error {
	page, err := strconv.Atoi(c.Query("page"))
	if err != nil {
		page = 1
		c.Query("page", "1")
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

	if c.Query("view") == "latest" {
		highlights, _, _, err := h.sortedHighlights(page, user, c.QueryInt("amount", latestHighlightsAmount), "created_at DESC", "all")
		if err != nil {
			return err
		}
		// Add completion status for latest highlights
		if session.ID > 0 {
			for i := range highlights {
				highlights[i] = h.readingRepository.Completed(int(session.ID), highlights[i])
			}
		}
		return h.latest(c, highlights, emailSendingConfigured)
	}

	sortBy := "created_at DESC"
	if c.Query("sort-by") == "highlighted-older-first" {
		sortBy = "created_at ASC"
	}
	filter := c.Query("filter")
	switch filter {
	case "highlights", "shared":
	default:
		filter = "all"
	}
	highlights, highlightsMap, totalHits, err := h.sortedHighlights(page, user, model.ResultsPerPage, sortBy, filter)
	if err != nil {
		return err
	}

	totalAll, err := h.hlRepository.Total(int(user.ID))
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	paginatedResults := result.NewPaginated(
		model.ResultsPerPage,
		page,
		totalHits,
		highlights,
	)

	// Add completion status for each document
	if session.ID > 0 {
		paginatedResults = h.readingRepository.CompletedPaginatedResult(int(session.ID), paginatedResults)
	}

	layout := "layout"
	if c.Query("view") == "list" {
		layout = ""
	}

	templateVars := fiber.Map{
		"Results":                paginatedResults,
		"Paginator":              view.Pagination(model.MaxPagesNavigator, paginatedResults, c.Queries()),
		"Title":                  "Highlights",
		"EmailSendingConfigured": emailSendingConfigured,
		"EmailFrom":              h.sender.From(),
		"WordsPerMinute":         h.wordsPerMinute,
		"URL":                    view.URL(c),
		"SortURL":                view.SortURL(c),
		"FilterURL":              view.FilterURL(c),
		"SortBy":                 c.Query("sort-by"),
		"HighlightsFilter":       filter,
		"HighlightsTotalAll":     totalAll,
		"Highlights":             highlightsMap,
		"ShowHighlightsFilter":   true,
		"AvailableLanguages":     c.Locals("AvailableLanguages"),
		"AdditionalSortOptions": []struct {
			Key   string
			Value string
		}{
			{"highlighted-newer-first", "latest highlights"},
			{"highlighted-older-first", "first highlights"},
		},
	}

	if c.Get("hx-request") == "true" {
		if err = c.Render("partials/highlights-list", templateVars); err != nil {
			log.Println(err)
			return fiber.ErrInternalServerError
		}
		return nil
	}
	if err = c.Render("highlight/index", templateVars, layout); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	return nil
}

func (h *Controller) sortedHighlights(page int, user *model.User, highlightsAmount int, sortBy, filter string) ([]index.Document, map[string]model.Highlight, int, error) {
	docsSortedByHighlightedDate, err := h.hlRepository.Highlights(int(user.ID), page, highlightsAmount, sortBy, filter)
	if err != nil {
		log.Println(err)
		return nil, nil, 0, fiber.ErrInternalServerError
	}

	if docsSortedByHighlightedDate.TotalPages() < page {
		page = docsSortedByHighlightedDate.TotalPages()
		docsSortedByHighlightedDate, err = h.hlRepository.Highlights(int(user.ID), page, highlightsAmount, sortBy, filter)
		if err != nil {
			log.Println(err)
			return nil, nil, 0, fiber.ErrInternalServerError
		}
	}

	highlights := make([]index.Document, 0, len(docsSortedByHighlightedDate.Hits()))
	highlightsMap := make(map[string]model.Highlight)
	for _, highlight := range docsSortedByHighlightedDate.Hits() {
		doc, err := h.idx.DocumentByID(highlight.Path)
		if err != nil {
			log.Println(err)
			return nil, nil, 0, fiber.ErrInternalServerError
		}
		if doc.ID == "" {
			continue
		}
		doc.Highlighted = true
		highlights = append(highlights, doc)
		highlightsMap[highlight.Path] = highlight
	}

	return highlights, highlightsMap, docsSortedByHighlightedDate.TotalHits(), nil
}

func (h *Controller) latest(c *fiber.Ctx, highlights []index.Document, emailSendingConfigured bool) error {
	err := c.Render("partials/latest-highlights", fiber.Map{
		"Highlights":             highlights,
		"EmailSendingConfigured": emailSendingConfigured,
		"EmailFrom":              h.sender.From(),
		"WordsPerMinute":         h.wordsPerMinute,
		"Amount":                 c.QueryInt("amount", latestHighlightsAmount),
	})
	if err != nil {
		log.Println(err)
	}

	return nil
}
