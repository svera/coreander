package webserver_test

import (
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/servicelinks"
)

type alternativeLinks struct{}

func (alternativeLinks) Author(author index.Author) servicelinks.Link {
	return servicelinks.Link{Source: "Another service", URL: "https://example.org/authors/" + author.Slug}
}

func TestAlternativeServiceLinks(t *testing.T) {
	source := &fakeBookSource{book: biblioreads.BookResponse{
		Rating: "4", ResourceID: "work", LegacyBookID: 1,
	}}
	app, idx := newServiceProvidersTestApp(t, source, nil, alternativeLinks{})
	for _, slug := range []string{"jane-doe", "john-doe"} {
		if err := idx.IndexAuthor(index.Author{
			Slug: slug, Name: slug, RetrievedOn: time.Now().UTC(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{
		"/authors/jane-doe", "/authors/jane-doe/summary",
		"/authors/jane-doe/summary?style=clear", "/documents/unlinked",
	} {
		page, err := goquery.NewDocumentFromReader(strings.NewReader(getBody(t, app, path)))
		if err != nil {
			t.Fatal(err)
		}
		link := page.Find(`a[href="https://example.org/authors/jane-doe"]`)
		if link.Length() != 1 || link.Text() != "Go to Another service author page" {
			t.Fatalf("%s: expected injected author service link, got %q", path, link.Text())
		}
		if page.Find(`img[src^="/images/goodreads.svg"]`).Length() != 0 {
			t.Fatalf("%s: unexpected Goodreads branding", path)
		}
	}
	for path, wantLinks := range map[string]int{"/documents/linked/score": 2, "/documents/linked/reviews": 1} {
		page, err := goquery.NewDocumentFromReader(strings.NewReader(getBody(t, app, path)))
		if err != nil {
			t.Fatal(err)
		}
		if page.Find(`a[href="https://www.goodreads.com/book/show/12207444-la-reina-dragon"]`).Length() != wantLinks {
			t.Fatalf("%s: BiblioReads must retain its Goodreads document URL regardless of the author link provider", path)
		}
	}
}

func TestServiceLinksWithoutReviewProvider(t *testing.T) {
	app, idx := newServiceProvidersTestApp(t, nil, nil, alternativeLinks{})
	if err := idx.IndexAuthor(index.Author{Slug: "jane-doe", Name: "Jane Doe", RetrievedOn: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	body := getBody(t, app, "/authors/jane-doe/summary")
	if !strings.Contains(body, "https://example.org/authors/jane-doe") {
		t.Fatal("links must not require a review provider")
	}
}
