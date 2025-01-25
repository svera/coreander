package view

import (
	"fmt"
	"html/template"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func ToQueryString(m map[string]string) template.URL {
	if len(m) == 0 {
		return ""
	}
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, fmt.Sprintf("%s=%s", k, url.QueryEscape(v)))
	}
	return template.URL(strings.Join(parts, "&"))
}

// URL returns the current URL along with the query string
func URL(c *fiber.Ctx) template.URL {
	url := c.Path()
	qs := string(c.Request().URI().QueryString())
	if qs != "" {
		url += "?" + qs
	}
	return template.URL(url)
}
