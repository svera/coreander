package servicelinks

import (
	"strings"

	"github.com/svera/coreander/v5/internal/index"
)

type Goodreads struct{}

func (Goodreads) Author(author index.Author) Link {
	return goodreadsLink(author.GoodReadsAuthorURL, "/author/show/")
}

func (Goodreads) Document(document index.Document) Link {
	return goodreadsLink(document.GoodReadsBookURL, "/book/show/")
}

func goodreadsLink(path, prefix string) Link {
	link := Link{Source: "Goodreads", Icon: "/images/goodreads.svg"}
	if id, ok := strings.CutPrefix(path, prefix); ok && id != "" {
		link.URL = "https://www.goodreads.com" + path
	}
	return link
}
