package index_test

import (
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/metadata"
)

func TestGoodReadsURLsAreStoredButNotSearchable(t *testing.T) {
	documentsIndexMem, err := bleve.NewMemOnly(index.CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}
	authorsIndexMem, err := bleve.NewMemOnly(index.CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}
	idx := index.NewBleve(documentsIndexMem, authorsIndexMem, nil, "", nil, index.Config{})

	const (
		bookURL   = "/book/show/12207444-zyxwvbook"
		authorURL = "/author/show/151155.Zyxwv_Author"
	)
	for _, lang := range []string{"es", ""} {
		doc := index.Document{
			ID:   "doc-" + lang,
			Slug: "doc-" + lang,
			Metadata: metadata.Metadata{
				Title:    "La Reina Dragon",
				Authors:  []string{"Alice Borchardt"},
				Language: lang,
				Format:   "EPUB",
			},
			AuthorsSlugs:       []string{"alice-borchardt"},
			GoodReadsBookURL:   bookURL,
			GoodReadsAuthorURL: authorURL,
		}
		if err := documentsIndexMem.Index(doc.ID, doc); err != nil {
			t.Fatal(err)
		}

		got, err := idx.Document(doc.Slug)
		if err != nil {
			t.Fatal(err)
		}
		if got.GoodReadsBookURL != bookURL || got.GoodReadsAuthorURL != authorURL {
			t.Errorf("language %q: expected stored URLs to be returned, got %q and %q", lang, got.GoodReadsBookURL, got.GoodReadsAuthorURL)
		}
	}

	for _, keywords := range []string{"zyxwvbook", "Zyxwv_Author", "12207444", "151155"} {
		res, err := idx.Search(index.SearchFields{Keywords: keywords}, 1, 10)
		if err != nil {
			t.Fatal(err)
		}
		if res.TotalHits() != 0 {
			t.Errorf("expected no results searching keywords %q, got %d", keywords, res.TotalHits())
		}
	}

	queries := map[string]bleve.SearchRequest{
		"book URL field term":   *bleve.NewSearchRequest(fieldTermQuery("GoodReadsBookURL", bookURL)),
		"author URL field term": *bleve.NewSearchRequest(fieldTermQuery("GoodReadsAuthorURL", authorURL)),
		"_all match":            *bleve.NewSearchRequest(bleve.NewMatchQuery("zyxwvbook")),
		"_all term":             *bleve.NewSearchRequest(bleve.NewTermQuery(bookURL)),
		"query string field":    *bleve.NewSearchRequest(bleve.NewQueryStringQuery(`GoodReadsBookURL:"` + bookURL + `"`)),
	}
	for name, req := range queries {
		res, err := documentsIndexMem.Search(&req)
		if err != nil {
			t.Fatal(err)
		}
		if res.Total != 0 {
			t.Errorf("%s: expected no hits, got %d", name, res.Total)
		}
	}
}

func fieldTermQuery(field, term string) *query.TermQuery {
	q := bleve.NewTermQuery(term)
	q.SetField(field)
	return q
}
