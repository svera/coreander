package servicelinks_test

import (
	"testing"

	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/servicelinks"
)

func TestGoodreadsLinks(t *testing.T) {
	provider := servicelinks.Goodreads{}
	for _, tt := range []struct {
		name   string
		path   string
		author bool
		want   string
	}{
		{"author", "/author/show/340641.Carmen_Posadas", true, "https://www.goodreads.com/author/show/340641.Carmen_Posadas"},
		{"document", "/book/show/5898652-la-cinta-roja", false, "https://www.goodreads.com/book/show/5898652-la-cinta-roja"},
		{"missing author", "", true, ""},
		{"missing document", "", false, ""},
		{"invalid author", "https://example.org", true, ""},
		{"invalid document", "/author/show/1", false, ""},
		{"empty identifier", "/book/show/", false, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			link := provider.Document(index.Document{GoodReadsBookURL: tt.path})
			if tt.author {
				link = provider.Author(index.Author{GoodReadsAuthorURL: tt.path})
			}
			if link.URL != tt.want || link.Source != "Goodreads" || link.Icon != "/images/goodreads.svg" {
				t.Fatalf("unexpected link: %+v", link)
			}
		})
	}
}
