package index_test

import (
	"testing"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/rickb777/date/v2"
	"github.com/svera/coreander/v5/internal/datasource/wikidata"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/metadata"
	"github.com/svera/coreander/v5/internal/precisiondate"
)

func TestSearchAuthors(t *testing.T) {
	authorsIndexMem, err := bleve.NewMemOnly(index.CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}
	documentsIndexMem, err := bleve.NewMemOnly(index.CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}

	idx := index.NewBleve(documentsIndexMem, authorsIndexMem, nil, "", nil, index.Config{})

	authors := []index.Author{
		{
			Slug:        "george-orwell",
			Name:        "George Orwell",
			BirthName:   "Eric Arthur Blair",
			Gender:      float64(wikidata.GenderMale),
			DateOfBirth: precisiondate.NewPrecisionDate("+1903-06-25T00:00:00Z", precisiondate.PrecisionDay),
			DateOfDeath: precisiondate.NewPrecisionDate("+1950-01-21T00:00:00Z", precisiondate.PrecisionDay),
			RetrievedOn: mustParseTime("2020-01-01T00:00:00Z"),
		},
		{
			Slug:        "jane-austen",
			Name:        "Jane Austen",
			Gender:      float64(wikidata.GenderFemale),
			DateOfBirth: precisiondate.NewPrecisionDate("+1775-12-16T00:00:00Z", precisiondate.PrecisionDay),
			DateOfDeath: precisiondate.NewPrecisionDate("+1817-07-18T00:00:00Z", precisiondate.PrecisionDay),
			RetrievedOn: mustParseTime("2020-01-01T00:00:00Z"),
		},
		{
			Slug:        "arturo-perez-reverte",
			Name:        "Arturo Pérez-Reverte",
			Gender:      float64(wikidata.GenderMale),
			DateOfBirth: precisiondate.NewPrecisionDate("+1951-11-24T00:00:00Z", precisiondate.PrecisionDay),
			RetrievedOn: mustParseTime("2020-01-01T00:00:00Z"),
		},
		{
			Slug:        "living-author",
			Name:        "Living Author",
			Gender:      float64(wikidata.GenderMale),
			DateOfBirth: precisiondate.NewPrecisionDate("+1990-01-01T00:00:00Z", precisiondate.PrecisionDay),
			RetrievedOn: mustParseTime("2020-01-01T00:00:00Z"),
		},
		{
			Slug:        "aristotle",
			Name:        "Aristotle",
			Gender:      float64(wikidata.GenderMale),
			DateOfBirth: precisiondate.NewPrecisionDate("-0384-01-01T00:00:00Z", precisiondate.PrecisionYear),
			DateOfDeath: precisiondate.NewPrecisionDate("-0322-01-01T00:00:00Z", precisiondate.PrecisionYear),
			RetrievedOn: mustParseTime("2020-01-01T00:00:00Z"),
		},
	}
	for _, author := range authors {
		if err := idx.IndexAuthor(author); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("alphabetical sort a-z", func(t *testing.T) {
		res, err := idx.SearchAuthors(index.AuthorSearchFields{SortBy: []string{"Slug"}}, 1, 10)
		if err != nil {
			t.Fatal(err)
		}
		hits := res.Hits()
		expected := []string{"aristotle", "arturo-perez-reverte", "george-orwell", "jane-austen", "living-author"}
		if len(hits) != len(expected) {
			t.Fatalf("expected %d results, got %d", len(expected), len(hits))
		}
		for i, slug := range expected {
			if hits[i].Slug != slug {
				t.Fatalf("position %d: expected %q, got %q", i, slug, hits[i].Slug)
			}
		}
	})

	t.Run("alphabetical sort z-a", func(t *testing.T) {
		res, err := idx.SearchAuthors(index.AuthorSearchFields{SortBy: []string{"-Slug"}}, 1, 10)
		if err != nil {
			t.Fatal(err)
		}
		hits := res.Hits()
		expected := []string{"living-author", "jane-austen", "george-orwell", "arturo-perez-reverte", "aristotle"}
		if len(hits) != len(expected) {
			t.Fatalf("expected %d results, got %d", len(expected), len(hits))
		}
		for i, slug := range expected {
			if hits[i].Slug != slug {
				t.Fatalf("position %d: expected %q, got %q", i, slug, hits[i].Slug)
			}
		}
	})

	t.Run("by name", func(t *testing.T) {
		res, err := idx.SearchAuthors(index.AuthorSearchFields{Name: "Orwell"}, 1, 10)
		if err != nil {
			t.Fatal(err)
		}
		hits := res.Hits()
		if res.TotalHits() != 1 || hits[0].Slug != "george-orwell" {
			t.Fatalf("expected George Orwell, got %#v", hits)
		}
	})

	t.Run("by name case insensitive", func(t *testing.T) {
		for _, name := range []string{"orwell", "ORWELL", "oRwElL"} {
			res, err := idx.SearchAuthors(index.AuthorSearchFields{Name: name}, 1, 10)
			if err != nil {
				t.Fatal(err)
			}
			hits := res.Hits()
			if res.TotalHits() != 1 || hits[0].Slug != "george-orwell" {
				t.Fatalf("search %q: expected George Orwell, got %#v", name, hits)
			}
		}
	})

	t.Run("by name unaccented and spaced", func(t *testing.T) {
		for _, name := range []string{"perez reverte", "Pérez-Reverte", "perez-reverte", "PEREZ REVERTE"} {
			res, err := idx.SearchAuthors(index.AuthorSearchFields{Name: name}, 1, 10)
			if err != nil {
				t.Fatal(err)
			}
			hits := res.Hits()
			if res.TotalHits() != 1 || hits[0].Slug != "arturo-perez-reverte" {
				t.Fatalf("search %q: expected Arturo Pérez-Reverte, got %#v", name, hits)
			}
		}
	})

	t.Run("by gender", func(t *testing.T) {
		female := float64(wikidata.GenderFemale)
		res, err := idx.SearchAuthors(index.AuthorSearchFields{Gender: &female}, 1, 10)
		if err != nil {
			t.Fatal(err)
		}
		hits := res.Hits()
		if res.TotalHits() != 1 || hits[0].Slug != "jane-austen" {
			t.Fatalf("expected Jane Austen, got %#v", hits)
		}
	})

	t.Run("by birth date range", func(t *testing.T) {
		from, _ := date.ParseISO("1900-01-01")
		to, _ := date.ParseISO("1920-12-31")
		res, err := idx.SearchAuthors(index.AuthorSearchFields{BirthDateFrom: from, BirthDateTo: to}, 1, 10)
		if err != nil {
			t.Fatal(err)
		}
		hits := res.Hits()
		if res.TotalHits() != 1 || hits[0].Slug != "george-orwell" {
			t.Fatalf("expected George Orwell, got %#v", hits)
		}
	})

	t.Run("by death date range", func(t *testing.T) {
		from, _ := date.ParseISO("1810-01-01")
		to, _ := date.ParseISO("1820-12-31")
		res, err := idx.SearchAuthors(index.AuthorSearchFields{DeathDateFrom: from, DeathDateTo: to}, 1, 10)
		if err != nil {
			t.Fatal(err)
		}
		hits := res.Hits()
		if res.TotalHits() != 1 || hits[0].Slug != "jane-austen" {
			t.Fatalf("expected Jane Austen, got %#v", hits)
		}
	})

	t.Run("by death date to excludes living authors", func(t *testing.T) {
		to, _ := date.ParseISO("2020-12-31")
		res, err := idx.SearchAuthors(index.AuthorSearchFields{DeathDateTo: to}, 1, 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, hit := range res.Hits() {
			if hit.Slug == "living-author" {
				t.Fatalf("living author should not match death date until filter, got %#v", res.Hits())
			}
		}
	})

	// Aristotle born year -384 (384 BC). On the number line: -400 < -384 < -300 < -100 < 0 < 1775 ...
	// "to -300" means upper bound = -300, so authors born at or before -300 (300 BC or earlier).
	// Aristotle (-384) is earlier than -300, so he qualifies.
	t.Run("by birth date to only (BC)", func(t *testing.T) {
		to, _ := date.ParseISO("-0300-01-01")
		res, err := idx.SearchAuthors(index.AuthorSearchFields{BirthDateTo: to}, 1, 10)
		if err != nil {
			t.Fatal(err)
		}
		hits := res.Hits()
		if res.TotalHits() != 1 || hits[0].Slug != "aristotle" {
			t.Fatalf("expected only Aristotle (born 384 BC), got %#v", hits)
		}
	})

	// "from -300" means lower bound = -300, so authors born at or after -300 (300 BC or later).
	// Aristotle (-384) is earlier than -300, so he should NOT be found.
	t.Run("by birth date from only (BC)", func(t *testing.T) {
		from, _ := date.ParseISO("-0300-01-01")
		res, err := idx.SearchAuthors(index.AuthorSearchFields{BirthDateFrom: from}, 1, 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, hit := range res.Hits() {
			if hit.Slug == "aristotle" {
				t.Fatalf("Aristotle (born 384 BC) should not match birth date from 300 BC, got %#v", res.Hits())
			}
		}
	})

	t.Run("by birth date range (BC)", func(t *testing.T) {
		from, _ := date.ParseISO("-0400-01-01")
		to, _ := date.ParseISO("-0300-01-01")
		res, err := idx.SearchAuthors(index.AuthorSearchFields{BirthDateFrom: from, BirthDateTo: to}, 1, 10)
		if err != nil {
			t.Fatal(err)
		}
		hits := res.Hits()
		if res.TotalHits() != 1 || hits[0].Slug != "aristotle" {
			t.Fatalf("expected only Aristotle (born 384 BC), got %#v", hits)
		}
	})

	t.Run("by document count", func(t *testing.T) {
		docs := []index.Document{
			{
				ID:           "doc-1",
				Slug:         "doc-1",
				AuthorsSlugs: []string{"george-orwell"},
				Metadata:     metadata.Metadata{Title: "1984", Authors: []string{"George Orwell"}},
			},
			{
				ID:           "doc-2",
				Slug:         "doc-2",
				AuthorsSlugs: []string{"george-orwell"},
				Metadata:     metadata.Metadata{Title: "Animal Farm", Authors: []string{"George Orwell"}},
			},
			{
				ID:           "doc-3",
				Slug:         "doc-3",
				AuthorsSlugs: []string{"jane-austen"},
				Metadata:     metadata.Metadata{Title: "Pride", Authors: []string{"Jane Austen"}},
			},
		}
		for _, doc := range docs {
			if err := documentsIndexMem.Index(doc.ID, doc); err != nil {
				t.Fatal(err)
			}
		}
		if err := idx.RebuildAuthorsFromDocuments(10); err != nil {
			t.Fatal(err)
		}

		moreFirst, err := idx.SearchAuthors(index.AuthorSearchFields{SortBy: []string{"-DocumentCount"}}, 1, 10)
		if err != nil {
			t.Fatal(err)
		}
		moreHits := moreFirst.Hits()
		if len(moreHits) != 5 ||
			moreHits[0].Slug != "george-orwell" ||
			moreHits[1].Slug != "jane-austen" {
			t.Fatalf("expected authors sorted by most documents first, got %#v", moreHits)
		}

		fewerFirst, err := idx.SearchAuthors(index.AuthorSearchFields{SortBy: []string{"DocumentCount"}}, 1, 10)
		if err != nil {
			t.Fatal(err)
		}
		fewerHits := fewerFirst.Hits()
		if len(fewerHits) != 5 ||
			fewerHits[3].Slug != "jane-austen" ||
			fewerHits[4].Slug != "george-orwell" {
			t.Fatalf("expected authors sorted by fewest documents first, got %#v", fewerHits)
		}
	})
}

func mustParseTime(value string) time.Time {
	t, _ := time.Parse(time.RFC3339, value)
	return t
}
