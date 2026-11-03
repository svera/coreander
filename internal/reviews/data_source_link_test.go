package reviews_test

import (
	"testing"

	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/reviews"
)

func TestBiblioReadsDataSourceLinks(t *testing.T) {
	provider := reviews.NewBiblioReads(nil, nil)
	author := provider.AuthorLink(index.Author{
		GoodReadsAuthorURL: "/author/show/123.Jane_Doe",
	}, "en")
	if author.URL != "https://www.goodreads.com/author/show/123.Jane_Doe" ||
		author.Source != "Goodreads" || author.Icon != "/images/goodreads.svg" || !author.IconInvertible {
		t.Fatalf("unexpected Goodreads author link: %+v", author)
	}

	document := provider.DocumentLink(index.Document{
		DataSourceLink: "/book/show/456.A_Book",
	})
	if document.URL != "https://www.goodreads.com/book/show/456.A_Book" ||
		document.Source != "Goodreads" || document.Icon != "/images/goodreads.svg" || !document.IconInvertible {
		t.Fatalf("unexpected Goodreads document link: %+v", document)
	}
}
