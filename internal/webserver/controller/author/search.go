package author

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

func (d *Controller) Search(c *fiber.Ctx) error {
	emailSendingConfigured := true
	if _, ok := d.sender.(*infrastructure.NoEmail); ok {
		emailSendingConfigured = false
	}

	var session model.Session
	if val, ok := c.Locals("Session").(model.Session); ok {
		session = val
	}

	if session.WordsPerMinute > 0 {
		d.config.WordsPerMinute = session.WordsPerMinute
	}

	var searchResults result.Paginated[[]index.Document]
	authorSlug := c.Params("slug")

	if authorSlug == "" {
		return fiber.ErrBadRequest
	}

	page, err := strconv.Atoi(c.Query("page"))
	if err != nil {
		page = 1
	}

	author, _ := d.idx.Author(authorSlug)
	if searchResults, err = d.idx.SearchByAuthor(authorSlug, page, model.ResultsPerPage); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	if session.ID > 0 {
		searchResults = d.hlRepository.HighlightedPaginatedResult(int(session.ID), searchResults)
	}

	err = c.Render("author/results", fiber.Map{
		"Author":                 author,
		"Results":                searchResults,
		"Paginator":              view.Pagination(model.MaxPagesNavigator, searchResults, map[string]string{}),
		"Title":                  fmt.Sprintf("Coreander - %s", author.Name),
		"EmailSendingConfigured": emailSendingConfigured,
		"EmailFrom":              d.sender.From(),
		"WordsPerMinute":         d.config.WordsPerMinute,
	}, "layout")

	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	return nil
}