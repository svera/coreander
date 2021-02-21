package webserver

import (
	"fmt"
	"html/template"
)

// Page holds the URL of a results page, and if that page is the current one being shown
type Page struct {
	Link      template.URL
	IsCurrent bool
}

// PagesNavigator contains all pages links, as well as links to the previous and next pages from the current one
type PagesNavigator struct {
	Pages        map[int]Page
	PreviousLink template.URL
	NextLink     template.URL
}

func pagination(size int, totalPages int, current int, searchType, keywords string) PagesNavigator {
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
			Link: template.URL(fmt.Sprintf("?%s=%s&page=%d", searchType, keywords, i)),
		}
		if i == current {
			p.IsCurrent = true
			if i > 1 {
				nav.PreviousLink = template.URL(fmt.Sprintf("?%s=%s&page=%d", searchType, keywords, i-1))
			}
			if i < totalPages {
				nav.NextLink = template.URL(fmt.Sprintf("?%s=%s&page=%d", searchType, keywords, i+1))
			}
		}
		nav.Pages[i] = p
	}
	return nav
}
