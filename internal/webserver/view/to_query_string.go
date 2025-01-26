package view

import (
	"fmt"
	"html/template"
	"net/url"
	"strings"
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
