package highlight

import (
	"log"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/svera/coreander/v4/internal/result"
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

	if c.Query("view") == "latest" {
		highlights, _, err := h.sortedHighlights(page, user, c.QueryInt("amount", latestHighlightsAmount), "created_at DESC", "all")
		if err != nil {
			return err
		}
		return h.latest(c, highlights)
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
	highlights, totalHits, err := h.sortedHighlights(page, user, model.ResultsPerPage, sortBy, filter)
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

	layout := "layout"
	if c.Query("view") == "list" {
		layout = ""
	}

	templateVars := fiber.Map{
		"Results":              paginatedResults,
		"Paginator":            view.Pagination(model.MaxPagesNavigator, paginatedResults, c.Queries()),
		"Title":                "Highlights",
		"EmailFrom":            h.sender.From(),
		"WordsPerMinute":       h.wordsPerMinute,
		"URL":                  view.URL(c),
		"SortURL":              view.BaseURLWithout(c, "sort-by", "page"),
		"FilterURL":            view.BaseURLWithout(c, "filter", "page"),
		"SortBy":               c.Query("sort-by"),
		"HighlightsFilter":     filter,
		"HighlightsTotalAll":   totalAll,
		"ShowHighlightsFilter": true,
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

func (h *Controller) sortedHighlights(page int, user *model.User, highlightsAmount int, sortBy, filter string) ([]model.Highlight, int, error) {
	docsSortedByHighlightedDate, err := h.hlRepository.Highlights(int(user.ID), page, highlightsAmount, sortBy, filter)
	if err != nil {
		log.Println(err)
		return nil, 0, fiber.ErrInternalServerError
	}

	if docsSortedByHighlightedDate.TotalPages() < page {
		page = docsSortedByHighlightedDate.TotalPages()
		docsSortedByHighlightedDate, err = h.hlRepository.Highlights(int(user.ID), page, highlightsAmount, sortBy, filter)
		if err != nil {
			log.Println(err)
			return nil, 0, fiber.ErrInternalServerError
		}
	}

	highlights := make([]model.Highlight, 0, len(docsSortedByHighlightedDate.Hits()))
	for _, highlight := range docsSortedByHighlightedDate.Hits() {
		doc, err := h.idx.DocumentByID(highlight.Path)
		if err != nil {
			log.Println(err)
			return nil, 0, fiber.ErrInternalServerError
		}
		if doc.ID == "" {
			continue
		}
		doc.Highlighted = true
		highlight.Document = doc
		highlights = append(highlights, highlight)
	}

	// Add completion status directly to embedded documents in highlights
	h.readingRepository.CompletedHighlights(int(user.ID), highlights)

	return highlights, docsSortedByHighlightedDate.TotalHits(), nil
}

func (h *Controller) latest(c *fiber.Ctx, highlights []model.Highlight) error {
	err := c.Render("partials/latest-highlights", fiber.Map{
		"Highlights":     highlights,
		"EmailFrom":      h.sender.From(),
		"WordsPerMinute": h.wordsPerMinute,
		"Amount":         c.QueryInt("amount", latestHighlightsAmount),
	})
	if err != nil {
		log.Println(err)
	}

	return nil
}
