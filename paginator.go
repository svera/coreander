package main

import "fmt"

type Page struct {
	Number    int
	Link      string
	IsCurrent bool
}

type PagesNavigator struct {
	Pages        []Page
	PreviousLink string
	NextLink     string
}

func pagination(size int, current int, search string) PagesNavigator {
	nav := PagesNavigator{
		Pages: make([]Page, size),
	}
	for i := range nav.Pages {
		nav.Pages[i].Number = i + 1
		nav.Pages[i].Link = fmt.Sprintf("?search=%s&page=%d", search, i+1)
		if i+1 == current {
			nav.Pages[i].IsCurrent = true
			if i > 0 {
				nav.PreviousLink = fmt.Sprintf("?search=%s&page=%d", search, i)
			}
			if i+1 < len(nav.Pages)-1 {
				nav.NextLink = fmt.Sprintf("?search=%s&page=%d", search, i+2)
			}
		}
	}
	return nav
}
