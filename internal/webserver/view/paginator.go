package view

import (
	"fmt"

	"github.com/svera/coreander/v4/internal/result"
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

func Pagination[T any](size int, results result.Paginated[T], params map[string]string) PagesNavigator {
	var nav PagesNavigator
	start := 1
	end := size
	if results.TotalPages() > size {
		nav = PagesNavigator{
			Pages: make(map[int]Page, size),
		}
		if results.Page() > size/2 {
			start = results.Page() - size/2
			end = (results.Page() + size/2)
			if end > results.TotalPages() {
				start = results.TotalPages() - size
				end = results.TotalPages()
			}
		}
	} else {
		nav = PagesNavigator{
			Pages: make(map[int]Page, results.TotalPages()),
		}
		end = results.TotalPages()
	}
	for i := start; i <= end; i++ {
		if params == nil {
			params = make(map[string]string, 1)
		}
		params["page"] = fmt.Sprintf("%d", i)
		p := Page{
			Link: fmt.Sprintf("?%s", ToQueryString(params)),
		}
		if i == results.Page() {
			p.IsCurrent = true
			if i > 1 {
				params["page"] = fmt.Sprintf("%d", i-1)
				nav.PreviousLink = fmt.Sprintf("?%s", ToQueryString(params))
			}
			if i < results.TotalPages() {
				params["page"] = fmt.Sprintf("%d", i+1)
				nav.NextLink = fmt.Sprintf("?%s", ToQueryString(params))
			}
		}
		nav.Pages[i] = p
	}
	return nav
}
