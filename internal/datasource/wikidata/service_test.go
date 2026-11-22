package wikidata

import (
	"reflect"
	"testing"

	gowikidata "github.com/Navid2zp/go-wikidata"
	"github.com/svera/coreander/v5/internal/datasource/model"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/precisiondate"
)

func TestNormalizeAuthorID(t *testing.T) {
	tests := []struct {
		input string
		want  string
		valid bool
	}{
		{"", "", true},
		{" \t ", "", true},
		{" Q1234 ", "Q1234", true},
		{"q001", "Q001", true},
		{"https://www.wikidata.org/wiki/Q1234", "Q1234", true},
		{"http://wikidata.org/entity/q1234/", "Q1234", true},
		{"https://www.wikidata.org/wiki/Q1234/extra", "", false},
		{"https://www.wikidata.org/wiki/Q1234//", "", false},
		{"https://www.wikidata.org/wiki/Q1234?x=1", "", false},
		{"https://www.wikidata.org/wiki/Q1234?", "", false},
		{"https://www.wikidata.org/wiki/Q1234#section", "", false},
		{"https://www.wikidata.org/wiki/Q1234#", "", false},
		{"https://user@www.wikidata.org/wiki/Q1234", "", false},
		{"https://www.wikidata.org.evil.example/wiki/Q1234", "", false},
		{"https://www.wikidata.org:443/wiki/Q1234", "", false},
		{"https://www.wikidata.org/wiki/%511234", "", false},
		{"https://www.wikidata.org/wiki/P1234", "", false},
		{"https://www.wikidata.org/w/index.php/Q1234", "", false},
		{"ftp://www.wikidata.org/wiki/Q1234", "", false},
		{"Q", "", false},
		{"Q12a", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := (WikidataSource{}).NormalizeAuthorID(tt.input)
			if (err == nil) != tt.valid || got != tt.want {
				t.Fatalf("NormalizeAuthorID(%q) = %q, %v; want %q, valid=%v", tt.input, got, err, tt.want, tt.valid)
			}
		})
	}
}

func TestAuthorLinks(t *testing.T) {
	author := Author{
		wikidataEntityId: "Q1234",
		website:          "https://author.example",
		wikipediaLink: map[string]string{
			"en": "https://en.wikipedia.org/wiki/Example",
			"es": "https://es.wikipedia.org/wiki/Ejemplo",
			"fr": "",
		},
	}
	want := map[string]string{
		"wikidata":     "Q1234",
		"website":      "https://author.example",
		"wikipedia:en": "https://en.wikipedia.org/wiki/Example",
		"wikipedia:es": "https://es.wikipedia.org/wiki/Ejemplo",
	}
	if got := author.Links(); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected author links: %+v", got)
	}
	links := author.Links()
	links["wikidata"] = "Q1"
	links["website"] = "https://changed.example"
	links["wikipedia:en"] = "https://changed.example/en"
	if got := author.Links(); !reflect.DeepEqual(got, want) {
		t.Fatalf("caller mutated author links: %+v", got)
	}
	for _, key := range []string{"wikidata", "website", "wikipedia:en", "wikipedia:de"} {
		if !author.OwnsLink(key) {
			t.Errorf("Wikidata should own %q, including missing language links", key)
		}
	}
	for _, key := range []string{"goodreads", "catalog", "wikipedia", "website:other"} {
		if author.OwnsLink(key) {
			t.Errorf("Wikidata should not own %q", key)
		}
	}
	if links := (Author{}).Links(); len(links) != 0 {
		t.Fatalf("expected no links for an empty author: %+v", links)
	}
}

func TestWikipediaDataSourceLink(t *testing.T) {
	source := NewWikidataSource(Gowikidata{})
	link := source.AuthorLink(index.Author{
		DataSourceLinks: map[string]string{
			"wikipedia:en": "https://en.wikipedia.org/wiki/Example",
			"wikipedia:es": "https://es.wikipedia.org/wiki/Ejemplo",
		},
	}, "es")
	if link.Source != "Wikipedia" || link.URL != "https://es.wikipedia.org/wiki/Ejemplo" ||
		link.IconClass != "bi-wikipedia" {
		t.Fatalf("unexpected Wikipedia data-source link: %+v", link)
	}
	if missing := source.AuthorLink(index.Author{}, "en"); missing.URL != "" {
		t.Fatalf("expected no link for missing Wikipedia URL, got %+v", missing)
	}
}

func TestAuthor(t *testing.T) {
	mockServer := NewMockServer(t, "fixtures")
	defer mockServer.Close()
	gowikidata.WikidataDomain = mockServer.URL

	for _, tcase := range testCases(t) {
		t.Run(tcase.name, func(t *testing.T) {

			wikidataSource := NewWikidataSource(Gowikidata{})

			author, err := wikidataSource.SearchAuthor(tcase.search, []string{"en"})
			if err != nil && !reflect.DeepEqual(tcase.expectedValue, Author{}) {
				t.Errorf("Error retrieving author: %v", err)
			}
			if reflect.DeepEqual(tcase.expectedValue, Author{}) {
				return
			}

			tcase.expectedValue.retrievedOn = author.RetrievedOn()
			if !reflect.DeepEqual(author, tcase.expectedValue) {
				t.Errorf("Wrong author\n\nexpected '%#v'\n\ngot '%#v'", tcase.expectedValue, author)
			}
		})
	}
}

type testCase struct {
	name          string
	search        string
	expectedValue Author
}

func testCases(t *testing.T) []testCase {
	return []testCase{
		{
			name:   "Author successfully retrieved",
			search: "Miguel",
			expectedValue: Author{
				birthName:        "Douglas Noël Adams",
				instanceOf:       InstanceHuman,
				wikidataEntityId: "Q1234",
				wikipediaLink:    make(map[string]string),
				description:      make(map[string]string),
				gender:           model.GenderMale,
				website:          "https://douglasadams.com",
				image:            "https://upload.wikimedia.org/wikipedia/commons/4/44/Duble_herma_of_Socrates_and_Seneca_Antikensammlung_Berlin_07.jpg",
				dateOfBirth:      precisiondate.NewPrecisionDate("-0004-00-00T00:00:00Z", precisiondate.PrecisionDecade),
				dateOfDeath:      precisiondate.NewPrecisionDate("+0065-04-12T00:00:00Z", precisiondate.PrecisionDay),
				pseudonyms:       []string{"David Agnew"},
			},
		},
		{
			name:          "Author not found",
			search:        "Eufrasio",
			expectedValue: Author{},
		},
		{
			name:          "Found entry is not human",
			search:        "Q1234",
			expectedValue: Author{},
		},
	}
}

func TestRetrieveAuthorsBatch(t *testing.T) {
	mockServer := NewMockServer(t, "fixtures")
	defer mockServer.Close()
	gowikidata.WikidataDomain = mockServer.URL

	source := NewWikidataSource(Gowikidata{})
	authors := make(map[string]model.Author)
	err := source.RetrieveAuthors(map[string][]string{
		"miguel": {"Q1234"},
	}, []string{"en"}, 0, func(slug string, a model.Author) error {
		authors[slug] = a
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(authors) != 1 {
		t.Fatalf("expected 1 author from batch retrieve, got %d", len(authors))
	}
	if authors["miguel"].Links()["wikidata"] != "Q1234" {
		t.Fatalf("expected Q1234, got %q", authors["miguel"].Links()["wikidata"])
	}
}
