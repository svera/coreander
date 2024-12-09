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
	gowiki "github.com/trietmn/go-wiki"
)

func (a *Controller) Search(c *fiber.Ctx) error {
	emailSendingConfigured := true
	if _, ok := a.sender.(*infrastructure.NoEmail); ok {
		emailSendingConfigured = false
	}

	var session model.Session
	if val, ok := c.Locals("Session").(model.Session); ok {
		session = val
	}

	if session.WordsPerMinute > 0 {
		a.config.WordsPerMinute = session.WordsPerMinute
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

	author, _ := a.idx.Author(authorSlug)
	if searchResults, err = a.idx.SearchByAuthor(authorSlug, page, model.ResultsPerPage); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	fmt.Printf("This is the page content: %v\n", d.info(c, author.Name))

	if session.ID > 0 {
		searchResults = d.hlRepository.HighlightedPaginatedResult(int(session.ID), searchResults)
	}

	err = c.Render("author/results", fiber.Map{
		"Author":                 author,
		"Results":                searchResults,
		"Paginator":              view.Pagination(model.MaxPagesNavigator, searchResults, map[string]string{}),
		"Title":                  fmt.Sprintf("Coreander - %s", author.Name),
		"EmailSendingConfigured": emailSendingConfigured,
		"EmailFrom":              a.sender.From(),
		"WordsPerMinute":         a.config.WordsPerMinute,
	}, "layout")

	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}
	return nil
}

func (d *Controller) info(c *fiber.Ctx, author string) string {
	gowiki.SetLanguage(c.Locals("Lang").(string))
	wikiPage, err := gowiki.GetPage(author, -1, false, true)
	if err != nil {
		fmt.Println(err)
	}

	// Get the content of the page
	content, err := wikiPage.GetContent()
	if err != nil {
		fmt.Println(err)
	}

	return content
}
