package view

import (
	"html/template"

	"github.com/gofiber/fiber/v2"
)

// URL returns the current URL along with the query string
func URL(c *fiber.Ctx) template.URL {
	url := c.Path()
	qs := string(c.Request().URI().QueryString())
	if qs != "" {
		url += "?" + qs
	}
	return template.URL(url)
}

func SortURL(c *fiber.Ctx) template.URL {
	url := c.Path()
	queries := c.Queries()
	delete(queries, "sort-by")
	delete(queries, "page")
	if len(queries) > 0 {
		return template.URL(url + "?" + string(ToQueryString(queries)+"&"))
	}
	return template.URL(url + "?")
}

func FilterURL(c *fiber.Ctx) template.URL {
	url := c.Path()
	queries := c.Queries()
	delete(queries, "filter")
	delete(queries, "page")
	if len(queries) > 0 {
		return template.URL(url + "?" + string(ToQueryString(queries)+"&"))
	}
	return template.URL(url + "?")
}
