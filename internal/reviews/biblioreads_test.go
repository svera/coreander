package reviews_test

import (
	"context"
	"errors"
	"testing"

	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/reviews"
)

type reviewLinkIndex struct {
	link string
	err  error
}

func (i *reviewLinkIndex) GoodReadsBook(context.Context, index.Document) (biblioreads.SearchResult, error) {
	return biblioreads.SearchResult{}, nil
}

func (i *reviewLinkIndex) StoreGoodReadsDetails(index.Document, string, biblioreads.BookResponse) error {
	return nil
}

func (i *reviewLinkIndex) UpdateDataSourceLink(_ string, link string) error {
	i.link = link
	return i.err
}

func TestBiblioReadsUpdateDocumentLink(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{name: "numeric ID", url: "https://www.goodreads.com/book/show/1368314", want: "/book/show/1368314"},
		{name: "dotted slug", url: "https://www.goodreads.com/book/show/1368314.El_Cercano_Oriente", want: "/book/show/1368314.El_Cercano_Oriente"},
		{name: "hyphenated slug", url: "https://www.goodreads.com/book/show/243175949-el-cercano-oriente", want: "/book/show/243175949-el-cercano-oriente"},
		{name: "normalize shared URL", url: " http://goodreads.com/book/show/1368314/?from_search=true#reviews ", want: "/book/show/1368314"},
		{name: "empty"},
		{name: "relative", url: "/book/show/1368314"},
		{name: "other domain", url: "https://example.com/book/show/1368314"},
		{name: "deceptive domain", url: "https://www.goodreads.com.example.com/book/show/1368314"},
		{name: "credentials", url: "https://reader@www.goodreads.com/book/show/1368314"},
		{name: "port", url: "https://www.goodreads.com:4000/book/show/1368314"},
		{name: "wrong scheme", url: "ftp://www.goodreads.com/book/show/1368314"},
		{name: "author", url: "https://www.goodreads.com/author/show/16667.Isaac_Asimov"},
		{name: "non-numeric ID", url: "https://www.goodreads.com/book/show/not-a-book"},
		{name: "zero ID", url: "https://www.goodreads.com/book/show/0"},
		{name: "extra path", url: "https://www.goodreads.com/book/show/1368314/other"},
		{name: "encoded path separator", url: "https://www.goodreads.com/book/show/1368314%2Fother"},
		{name: "malformed URL", url: "https://%zz/book/show/1368314"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := &reviewLinkIndex{}
			provider := reviews.NewBiblioReads(nil, idx)
			err := provider.UpdateDocumentLink(index.Document{ID: "doc-id"}, tt.url)
			if tt.want == "" {
				if !errors.Is(err, reviews.ErrInvalidDocumentLink) {
					t.Fatalf("expected invalid-link error, got %v", err)
				}
				if idx.link != "" {
					t.Fatalf("invalid link was passed to index: %q", idx.link)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if idx.link != tt.want {
				t.Fatalf("normalized link = %q, want %q", idx.link, tt.want)
			}
		})
	}
}

func TestBiblioReadsUpdateDocumentLinkMissingDocument(t *testing.T) {
	idx := &reviewLinkIndex{err: index.ErrDocumentNotFound}
	provider := reviews.NewBiblioReads(nil, idx)
	err := provider.UpdateDocumentLink(index.Document{ID: "missing"}, "https://www.goodreads.com/book/show/1368314")
	if !errors.Is(err, reviews.ErrDocumentNotFound) {
		t.Fatalf("expected missing-document error, got %v", err)
	}
}
