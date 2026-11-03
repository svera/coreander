package index_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/metadata"
)

func mustGoodReadsData(t *testing.T, document index.Document) biblioreads.DocumentData {
	t.Helper()
	data, err := index.DataSourceData[biblioreads.DocumentData](document, biblioreads.Source)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func withGoodReadsData(t *testing.T, document index.Document, data biblioreads.DocumentData) index.Document {
	t.Helper()
	if err := index.SetDataSourceData(&document, biblioreads.Source, data); err != nil {
		t.Fatal(err)
	}
	return document
}

func TestReviewScoreSorting(t *testing.T) {
	docs := []index.Document{
		{ID: "high", Slug: "high", DataSourceScoreValue: 4.75},
		{ID: "unrated", Slug: "unrated"},
		{ID: "low", Slug: "low", DataSourceScoreValue: 3.25},
	}
	for i := range docs {
		docs[i].Metadata = metadata.Metadata{Title: docs[i].Slug, Authors: []string{"Writer"}, Format: "EPUB", Series: "Series"}
		docs[i].AuthorsSlugs = []string{"writer"}
		docs[i].SeriesSlug = "series"
		docs[i].TextRankPhrases = []string{"atomic bomb", "los alamos", "manhattan project"}
	}
	reference := index.Document{
		ID: "reference", Slug: "reference",
		Metadata:        metadata.Metadata{Title: "Reference", Authors: []string{"Other"}, Format: "EPUB"},
		AuthorsSlugs:    []string{"other"},
		TextRankPhrases: docs[0].TextRankPhrases,
	}
	idx := newTextRankTestIndex(t, append(docs, reference))
	t.Cleanup(func() { _ = idx.Close() })

	for _, scope := range []string{"search", "author", "series", "similar"} {
		for _, direction := range []string{"asc", "desc"} {
			t.Run(scope+"/"+direction, func(t *testing.T) {
				key := "DataSourceScoreValue"
				want := []string{"low", "high"}
				if direction == "desc" {
					key = "-" + key
					slices.Reverse(want)
				}
				want = append(want, "unrated")
				fields := index.SearchFields{SortBy: []string{key}}
				switch scope {
				case "search":
					fields.Keywords = "writer"
				case "author":
					fields.AuthorSlug = "writer"
				case "series":
					fields.Keywords = "series"
				case "similar":
					fields.SimilarTo = "reference"
				}
				var got []string
				for page := 1; page <= 2; page++ {
					switch scope {
					case "author":
						res, err := idx.SearchByAuthor(fields, page, 2)
						if err != nil {
							t.Fatal(err)
						}
						got = append(got, slugsOf(res.Hits())...)
					case "series":
						res, err := idx.SearchBySeries(fields, page, 2)
						if err != nil {
							t.Fatal(err)
						}
						got = append(got, slugsOf(res.Hits())...)
					default:
						res, err := idx.Search(fields, page, 2)
						if err != nil {
							t.Fatal(err)
						}
						got = append(got, slugsOf(res.Hits())...)
					}
				}
				if !slices.Equal(got, want) {
					t.Fatalf("got %v, want %v", got, want)
				}
			})
		}
	}
}

func TestReviewFieldsAndProviderMetadataAreStoredButNotSearchable(t *testing.T) {
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
		doc := withGoodReadsData(t, index.Document{
			ID:   "doc-" + lang,
			Slug: "doc-" + lang,
			Metadata: metadata.Metadata{
				Title:    "La Reina Dragon",
				Authors:  []string{"Alice Borchardt"},
				Language: lang,
				Format:   "EPUB",
			},
			AuthorsSlugs: []string{"alice-borchardt"},

			DataSourceScoreValue: 4.25,
		}, biblioreads.DocumentData{URL: bookURL, RatingCount: "2285", ResourceID: resourceID, LegacyBookID: legacyBookID})

		if err := index.IndexDocument(documentsIndexMem, doc.ID, doc); err != nil {
			t.Fatal(err)
		}

		got, err := idx.Document(doc.Slug)
		if err != nil {
			t.Fatal(err)
		}
		if mustGoodReadsData(t, got).URL != bookURL {
			t.Errorf("language %q: expected stored book URL, got %q", lang, mustGoodReadsData(t, got).URL)
		}
		if got.DataSourceScoreValue != 4.25 {
			t.Errorf("language %q: expected stored rating, got %v", lang, got.DataSourceScoreValue)
		}
		if mustGoodReadsData(t, got).RatingCount != "2285" {
			t.Errorf("language %q: expected stored rating count, got %q", lang, mustGoodReadsData(t, got).RatingCount)
		}
		if mustGoodReadsData(t, got).ResourceID != resourceID || mustGoodReadsData(t, got).LegacyBookID != legacyBookID {
			t.Errorf("language %q: expected stored identifiers, got %q and %d", lang, mustGoodReadsData(t, got).ResourceID, mustGoodReadsData(t, got).LegacyBookID)
		}
	}
	if err := index.IndexDocument(documentsIndexMem, "unscored", index.Document{
		ID:   "unscored",
		Slug: "unscored",
		Metadata: metadata.Metadata{
			Title:  "Unscored document",
			Format: "EPUB",
		},
	}); err != nil {
		t.Fatal(err)
	}

	for _, keywords := range []string{"zyxwvbook", "Zyxwv_Author", "12207444", "151155", "zyxwvresource", resourceID, "4.25", "2285"} {
		res, err := idx.Search(index.SearchFields{Keywords: keywords}, 1, 10)
		if err != nil {
			t.Fatal(err)
		}
		if res.TotalHits() != 0 {
			t.Errorf("expected no results searching keywords %q, got %d", keywords, res.TotalHits())
		}
	}
	filtered, err := idx.Search(index.SearchFields{DataSourceScoreFrom: 4.2, DataSourceScoreTo: 4.3}, 1, 10)
	if err != nil || filtered.TotalHits() != 2 {
		t.Errorf("expected both documents in the review score range, got %d results and error %v", filtered.TotalHits(), err)
	}
	filtered, err = idx.Search(index.SearchFields{DataSourceScoreFrom: 4.3}, 1, 10)
	if err != nil || filtered.TotalHits() != 0 {
		t.Errorf("expected no documents above the review score, got %d results and error %v", filtered.TotalHits(), err)
	}
	filtered, err = idx.Search(index.SearchFields{DataSourceScoreNone: true}, 1, 10)
	if err != nil || filtered.TotalHits() != 1 || filtered.Hits()[0].DataSourceScoreValue != 0 {
		t.Errorf("expected only the unscored document, got %d results and error %v", filtered.TotalHits(), err)
	}

	queries := map[string]bleve.SearchRequest{
		"provider data field term": *bleve.NewSearchRequest(fieldTermQuery("DataSources", bookURL)),
		"provider URL field term":  *bleve.NewSearchRequest(fieldTermQuery("DataSources.goodreads.url", bookURL)),
		"author URL field term":    *bleve.NewSearchRequest(fieldTermQuery("GoodReadsAuthorURL", authorURL)),
		"resource ID field term":   *bleve.NewSearchRequest(fieldTermQuery("DataSources.goodreads.resourceId", resourceID)),
		"legacy ID numeric field":  *bleve.NewSearchRequest(legacyBookIDQuery(legacyBookID)),
		"_all match":               *bleve.NewSearchRequest(bleve.NewMatchQuery("zyxwvbook")),
		"_all term":                *bleve.NewSearchRequest(bleve.NewTermQuery(bookURL)),
		"query string field":       *bleve.NewSearchRequest(bleve.NewQueryStringQuery(`DataSources:"` + bookURL + `"`)),
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

func TestReviewScoreSortingMissingFieldsAndTies(t *testing.T) {
	documents, err := bleve.NewMemOnly(index.CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}
	authors, err := bleve.NewMemOnly(index.CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}
	idx := index.NewBleve(documents, authors, nil, "", nil, index.Config{})
	t.Cleanup(func() { _ = idx.Close() })
	for _, doc := range []map[string]any{
		{"Slug": "unrated-zero", "DataSourceScoreValue": 0.0, "SeriesIndex": 2.0},
		{"Slug": "high", "DataSourceScoreValue": 4.75, "SeriesIndex": 1.0},
		{"Slug": "unrated-missing", "SeriesIndex": 1.0},
		{"Slug": "low-second", "DataSourceScoreValue": 3.25, "SeriesIndex": 2.0},
		{"Slug": "low-first", "DataSourceScoreValue": 3.25, "SeriesIndex": 1.0},
	} {
		for _, field := range []string{"Title", "Description", "Series", "SeriesSlug", "Format"} {
			doc[field] = ""
		}
		doc["Words"], doc["Pages"] = 0.0, 0.0
		if err := documents.Index(doc["Slug"].(string), doc); err != nil {
			t.Fatal(err)
		}
	}
	for _, direction := range []string{"asc", "desc"} {
		t.Run(direction, func(t *testing.T) {
			key := "DataSourceScoreValue"
			want := []string{"low-first", "low-second", "high", "unrated-missing", "unrated-zero"}
			if direction == "desc" {
				key = "-" + key
				want = []string{"high", "low-first", "low-second", "unrated-missing", "unrated-zero"}
			}
			var got []string
			for page := 1; page <= 3; page++ {
				res, err := idx.Search(index.SearchFields{SortBy: []string{key, "SeriesIndex"}}, page, 2)
				if err != nil {
					t.Fatal(err)
				}
				if res.TotalHits() != len(want) {
					t.Errorf("got %d hits, want %d", res.TotalHits(), len(want))
				}
				got = append(got, slugsOf(res.Hits())...)
			}
			if !slices.Equal(got, want) {
				t.Fatalf("got %v, want %v", got, want)
			}
		})
	}
}

func legacyBookIDQuery(id int) *query.NumericRangeQuery {
	value := float64(id)
	q := bleve.NewNumericRangeQuery(&value, &value)
	q.SetField("DataSources.goodreads.legacyBookId")
	return q
}

func TestAuthorDataSourceLinksAreStoredButNotSearchable(t *testing.T) {
	authors, err := bleve.NewMemOnly(index.CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}
	defer authors.Close()
	idx := index.NewBleve(nil, authors, nil, "", nil, index.Config{})
	const authorURL = "/author/show/151155.Zyxwv_Author"
	links := map[string]string{
		"goodreads": authorURL,
		"other":     "https://example.org/Zyxwv_Author",
	}
	if err := idx.IndexAuthor(index.Author{Slug: "alice", Name: "Alice Borchardt", DataSourceLinks: links}); err != nil {
		t.Fatal(err)
	}
	author, err := idx.Author("alice", "")
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(author.DataSourceLinks, links) {
		t.Fatalf("expected stored author links, got %+v", author.DataSourceLinks)
	}
	for _, q := range []query.Query{
		fieldTermQuery("DataSourceLinks", authorURL),
		fieldTermQuery("DataSourceLinks.goodreads", authorURL),
		fieldTermQuery("DataSourceLinks.other", links["other"]),
		bleve.NewTermQuery(authorURL),
		bleve.NewMatchQuery("Zyxwv_Author"),
		bleve.NewQueryStringQuery(`DataSourceLinks:"` + authorURL + `"`),
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
