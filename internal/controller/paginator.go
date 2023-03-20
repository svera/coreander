package controller

import (
	"fmt"
	"net/url"
	"strings"
)

// Page holds the URL of a results page, and if that page is the current one being shown
type Page struct {
	Link      string
	IsCurrent bool
}

// PagesNavigator contains all pages links, as well as links to the previous and next pages from the current one
type PagesNavigator struct {
	Pages        map[int]Page
	PreviousLink string
	NextLink     string
}

func pagination(size int, totalPages int, current int, params map[string]string) PagesNavigator {
	var nav PagesNavigator
	start := 1
	end := size
	if totalPages > size {
		nav = PagesNavigator{
			Pages: make(map[int]Page, size),
		}
		if current > size/2 {
			start = current - size/2
			end = (current + size/2)
			if end > totalPages {
				start = totalPages - size
				end = totalPages
			}
		}
	} else {
		nav = PagesNavigator{
			Pages: make(map[int]Page, totalPages),
		}
		end = totalPages
	}
	for i := start; i <= end; i++ {
		p := Page{
			Link: fmt.Sprintf("?%spage=%d", toQueryString(params), i),
		}
		if i == current {
			p.IsCurrent = true
			if i > 1 {
				nav.PreviousLink = fmt.Sprintf("?%spage=%d", toQueryString(params), i-1)
			}
			if i < totalPages {
				nav.NextLink = fmt.Sprintf("?%spage=%d", toQueryString(params), i+1)
			}
		}
		nav.Pages[i] = p
	}
	return nav
}

func toQueryString(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, fmt.Sprintf("%s=%s", k, url.QueryEscape(v)))
	}
	return strings.Join(parts, "&") + "&"
}
