package webserver

import (
	"fmt"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
)

func routeSearch(c *fiber.Ctx, idx Reader, version string, emailSendingConfigured bool) error {
	lang := c.Params("lang")

	if lang != "es" && lang != "en" {
		return fiber.ErrNotFound
	}

	page, err := strconv.Atoi(c.Query("page"))
	if err != nil {
		page = 1
	}

	var claims jwt.MapClaims
	if c.Locals("user") != nil {
		user := c.Locals("user").(*jwt.Token)
		claims = user.Claims.(jwt.MapClaims)
	}
	fmt.Printf("%v", claims)
	var keywords string
	var searchResults *Result

	keywords = c.Query("search")
	if keywords != "" {
		searchResults, err = idx.Search(keywords, page, resultsPerPage)
		if err != nil {
			return fiber.ErrInternalServerError
		}

		return c.Render("results", fiber.Map{
			"Lang":                   lang,
			"Keywords":               keywords,
			"Results":                searchResults.Hits,
			"Total":                  searchResults.TotalHits,
			"Paginator":              pagination(maxPagesNavigator, searchResults.TotalPages, searchResults.Page, "search", keywords),
			"Title":                  "Search results",
			"Version":                version,
			"EmailSendingConfigured": emailSendingConfigured,
			"Claims":                 claims,
		}, "layout")
	}
	count, err := idx.Count()
	if err != nil {
		return fiber.ErrInternalServerError
	}
	return c.Render("index", fiber.Map{
		"Lang":    lang,
		"Count":   count,
		"Title":   "Coreander",
		"Version": version,
	}, "layout")
}
