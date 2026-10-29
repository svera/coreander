package index_test

import (
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/metadata"
)

func TestGoodReadsFieldsAreStoredButNotSearchable(t *testing.T) {
	documentsIndexMem, err := bleve.NewMemOnly(index.CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}
	authorsIndexMem, err := bleve.NewMemOnly(index.CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}
	idx := index.NewBleve(documentsIndexMem, authorsIndexMem, nil, "", nil, index.Config{})
	t.Cleanup(func() { _ = idx.Close() })

	const (
		bookURL      = "/book/show/12207444-zyxwvbook"
		authorURL    = "/author/show/151155.Zyxwv_Author"
		resourceID   = "kca://work/amzn1.gr.work.v1.zyxwvresource"
		legacyBookID = 12207444
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
			AuthorsSlugs:          []string{"alice-borchardt"},
			GoodReadsBookURL:      bookURL,
			GoodReadsResourceID:   resourceID,
			GoodReadsLegacyBookID: legacyBookID,
		}
		if err := documentsIndexMem.Index(doc.ID, doc); err != nil {
			t.Fatal(err)
		}

		got, err := idx.Document(doc.Slug)
		if err != nil {
			t.Fatal(err)
		}
		if got.GoodReadsBookURL != bookURL {
			t.Errorf("language %q: expected stored book URL, got %q", lang, got.GoodReadsBookURL)
		}
		if got.GoodReadsResourceID != resourceID || got.GoodReadsLegacyBookID != legacyBookID {
			t.Errorf("language %q: expected stored identifiers, got %q and %d", lang, got.GoodReadsResourceID, got.GoodReadsLegacyBookID)
		}
	}

	for _, keywords := range []string{"zyxwvbook", "Zyxwv_Author", "12207444", "151155", "zyxwvresource", resourceID} {
		res, err := idx.Search(index.SearchFields{Keywords: keywords}, 1, 10)
		if err != nil {
			t.Fatal(err)
		}
		if res.TotalHits() != 0 {
			t.Errorf("expected no results searching keywords %q, got %d", keywords, res.TotalHits())
		}
	}

	queries := map[string]bleve.SearchRequest{
		"book URL field term":     *bleve.NewSearchRequest(fieldTermQuery("GoodReadsBookURL", bookURL)),
		"author URL field term":   *bleve.NewSearchRequest(fieldTermQuery("GoodReadsAuthorURL", authorURL)),
		"resource ID field term":  *bleve.NewSearchRequest(fieldTermQuery("GoodReadsResourceID", resourceID)),
		"legacy ID numeric field": *bleve.NewSearchRequest(legacyBookIDQuery(legacyBookID)),
		"_all match":              *bleve.NewSearchRequest(bleve.NewMatchQuery("zyxwvbook")),
		"_all term":               *bleve.NewSearchRequest(bleve.NewTermQuery(bookURL)),
		"query string field":      *bleve.NewSearchRequest(bleve.NewQueryStringQuery(`GoodReadsBookURL:"` + bookURL + `"`)),
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

func legacyBookIDQuery(id int) *query.NumericRangeQuery {
	value := float64(id)
	q := bleve.NewNumericRangeQuery(&value, &value)
	q.SetField("GoodReadsLegacyBookID")
	return q
}

func TestAuthorGoodReadsURLIsStoredButNotSearchable(t *testing.T) {
	authors, err := bleve.NewMemOnly(index.CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}
	defer authors.Close()
	const authorURL = "/author/show/151155.Zyxwv_Author"
	if err := authors.Index("alice", index.Author{Slug: "alice", Name: "Alice Borchardt", GoodReadsAuthorURL: authorURL}); err != nil {
		t.Fatal(err)
	}
	req := bleve.NewSearchRequest(bleve.NewDocIDQuery([]string{"alice"}))
	req.Fields = []string{"GoodReadsAuthorURL"}
	result, err := authors.Search(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 1 || result.Hits[0].Fields["GoodReadsAuthorURL"] != authorURL {
		t.Fatalf("expected stored author URL, got %+v", result.Hits)
	}
	for _, q := range []query.Query{
		fieldTermQuery("GoodReadsAuthorURL", authorURL),
		bleve.NewTermQuery(authorURL),
		bleve.NewMatchQuery("Zyxwv_Author"),
		bleve.NewQueryStringQuery(`GoodReadsAuthorURL:"` + authorURL + `"`),
	} {
		result, err := authors.Search(bleve.NewSearchRequest(q))
		if err != nil {
			t.Fatal(err)
		}
		if result.Total != 0 {
			t.Errorf("author URL must not affect search results, got %d hits", result.Total)
		}
	}
}

func fieldTermQuery(field, term string) *query.TermQuery {
	q := bleve.NewTermQuery(term)
	q.SetField(field)
	return q
}

func TestDocumentsMappingHasNoAuthorURL(t *testing.T) {
	m := index.CreateDocumentsMapping().(*mapping.IndexMappingImpl)
	for name, documentMapping := range m.TypeMapping {
		if _, exists := documentMapping.Properties["GoodReadsAuthorURL"]; exists {
			t.Errorf("mapping %q still contains an author URL field", name)
		}
	}
	if _, exists := m.DefaultMapping.Properties["GoodReadsAuthorURL"]; exists {
		t.Error("default mapping still contains an author URL field")
	}
}
