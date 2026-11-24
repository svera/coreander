package wikidata

import (
	"reflect"
	"testing"

	gowikidata "github.com/Navid2zp/go-wikidata"
	"github.com/rickb777/date/v2"
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
		DataSourceReferences: map[string]string{
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

			author, err := wikidataSource.SearchAuthor(tcase.search, []string{"en"}, 0)
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
	err := source.RetrieveAuthors(map[string]model.AuthorCandidates{
		"miguel": {IDs: []string{"Q1234"}},
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

type candidateWikidata struct {
	entities map[string]gowikidata.Entity
}

func (w candidateWikidata) NewSearch(string, string) (SearchEntitiesRequest, error) {
	return candidateSearch{}, nil
}
func (w candidateWikidata) NewGetEntities([]string) (GetEntitiesRequest, error) {
	return candidateEntities{w.entities}, nil
}

type candidateSearch struct{}

func (candidateSearch) Get() (*gowikidata.SearchEntitiesResponse, error) {
	return &gowikidata.SearchEntitiesResponse{SearchResult: []gowikidata.SearchEntity{{ID: "Q1"}, {ID: "Q2"}}}, nil
}

type candidateEntities struct{ entities map[string]gowikidata.Entity }

func (candidateEntities) SetProps([]string)     {}
func (candidateEntities) SetLanguages([]string) {}
func (c candidateEntities) Get() (*map[string]gowikidata.Entity, error) {
	return &c.entities, nil
}

func testPerson(birth string, precision float64, rank string) gowikidata.Entity {
	var instance, dateClaim gowikidata.Claim
	instance.Rank = rank
	instance.MainSnak.DataValue.Value.ValueFields.ID = qidInstanceOfHuman
	dateClaim.Rank = "normal"
	dateClaim.MainSnak.DataValue.Value.ValueFields.Time = birth
	dateClaim.MainSnak.DataValue.Value.ValueFields.Precision = precision
	claims := map[string][]gowikidata.Claim{propertyInstanceOf: {instance}}
	if birth != "" {
		claims[propertyDateOfBirth] = []gowikidata.Claim{dateClaim}
	}
	return gowikidata.Entity{Claims: claims}
}

func TestAuthorSearchPublicationConstraint(t *testing.T) {
	cutoff := date.New(1900, 6, 15)
	tests := []struct {
		name      string
		birth     string
		precision float64
		cutoff    date.Date
		want      string
	}{
		{"later year", "+1901-01-01T00:00:00Z", precisiondate.PrecisionDay, cutoff, "Q2"},
		{"later day", "+1900-06-16T00:00:00Z", precisiondate.PrecisionDay, cutoff, "Q2"},
		{"same day", "+1900-06-15T00:00:00Z", precisiondate.PrecisionDay, cutoff, "Q1"},
		{"same month", "+1900-06-00T00:00:00Z", precisiondate.PrecisionMonth, cutoff, "Q1"},
		{"later month", "+1900-07-00T00:00:00Z", precisiondate.PrecisionMonth, cutoff, "Q2"},
		{"same year", "+1900-00-00T00:00:00Z", precisiondate.PrecisionYear, cutoff, "Q1"},
		{"later year precision", "+1901-00-00T00:00:00Z", precisiondate.PrecisionYear, cutoff, "Q2"},
		{"unknown birth", "", 0, cutoff, "Q1"},
		{"coarse birth", "+1900-00-00T00:00:00Z", precisiondate.PrecisionDecade, cutoff, "Q1"},
		{"malformed birth", "2T00:00:00Z", precisiondate.PrecisionDay, cutoff, "Q1"},
		{"BCE birth after publication", "-0100-06-16T00:00:00Z", precisiondate.PrecisionDay, date.New(-100, 6, 15), "Q2"},
		{"BCE month after publication", "-0100-07-00T00:00:00Z", precisiondate.PrecisionMonth, date.New(-100, 6, 15), "Q2"},
		{"unknown publication", "+2000-01-01T00:00:00Z", precisiondate.PrecisionDay, date.Zero, "Q1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := NewWikidataSource(candidateWikidata{entities: map[string]gowikidata.Entity{
				"Q1": testPerson(tt.birth, tt.precision, "preferred"),
				"Q2": testPerson("", 0, "normal"),
			}})
			author, err := source.SearchAuthor("Example", []string{"en"}, tt.cutoff)
			if err != nil || author == nil || author.Links()["wikidata"] != tt.want {
				t.Fatalf("SearchAuthor = %v, %v; want %s", author, err, tt.want)
			}
			var batched model.Author
			err = source.RetrieveAuthors(map[string]model.AuthorCandidates{
				"example": {IDs: []string{"Q1", "Q2"}, LatestPublicationDate: tt.cutoff},
			}, nil, 0, func(_ string, author model.Author) error {
				batched = author
				return nil
			})
			if err != nil || batched == nil || batched.Links()["wikidata"] != tt.want {
				t.Fatalf("RetrieveAuthors = %v, %v; want %s", batched, err, tt.want)
			}
		})
	}
}

func TestAuthorSearchNoCompatibleCandidatesAndExplicitID(t *testing.T) {
	source := NewWikidataSource(candidateWikidata{entities: map[string]gowikidata.Entity{
		"Q1": testPerson("+2000-01-01T00:00:00Z", precisiondate.PrecisionDay, "normal"),
		"Q2": testPerson("+2001-01-01T00:00:00Z", precisiondate.PrecisionDay, "normal"),
	}})
	author, err := source.SearchAuthor("Example", nil, date.New(1900, 12, 31))
	if err != nil || author != nil {
		t.Fatalf("expected no compatible candidate, got %v, %v", author, err)
	}
	var batched model.Author
	err = source.RetrieveAuthors(map[string]model.AuthorCandidates{
		"example": {IDs: []string{"Q1", "Q2"}, LatestPublicationDate: date.New(1900, 12, 31)},
	}, nil, 0, func(_ string, author model.Author) error {
		batched = author
		return nil
	})
	if err != nil || batched != nil {
		t.Fatalf("expected no compatible batched candidate, got %v, %v", batched, err)
	}
	author, err = source.RetrieveAuthor([]string{"Q1"}, nil)
	if err != nil || author == nil || author.Links()["wikidata"] != "Q1" {
		t.Fatalf("explicit ID should remain available, got %v, %v", author, err)
	}
}
