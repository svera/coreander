package wikidata

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	gowikidata "github.com/Navid2zp/go-wikidata"
	"github.com/svera/coreander/v5/internal/datasource/model"
	"github.com/svera/coreander/v5/internal/precisiondate"
)

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
				gender:           GenderMale,
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
	if authors["miguel"].SourceID() != "Q1234" {
		t.Fatalf("expected Q1234, got %q", authors["miguel"].SourceID())
	}
}

// fakeGetEntitiesRequest lets a test control whether a wbgetentities batch
// call succeeds or fails, without going through a real HTTP round trip.
type fakeGetEntitiesRequest struct {
	err error
}

func (f *fakeGetEntitiesRequest) SetProps(_ []string)     {}
func (f *fakeGetEntitiesRequest) SetLanguages(_ []string) {}
func (f *fakeGetEntitiesRequest) Get() (*map[string]gowikidata.Entity, error) {
	if f.err != nil {
		return nil, f.err
	}
	entities := make(map[string]gowikidata.Entity)
	return &entities, nil
}

// fakeWikidata fails the first wbgetentities batch it receives and lets
// every subsequent one succeed (with empty results), to simulate a
// transient Wikidata failure affecting a single chunk of a larger request.
type fakeWikidata struct {
	batches int
}

func (f *fakeWikidata) NewSearch(string, string) (SearchEntitiesRequest, error) {
	return nil, errors.New("not used in this test")
}

func (f *fakeWikidata) NewGetEntities(ids []string) (GetEntitiesRequest, error) {
	f.batches++
	if f.batches == 1 {
		return &fakeGetEntitiesRequest{err: errors.New("simulated network error")}, nil
	}
	return &fakeGetEntitiesRequest{}, nil
}

func TestRetrieveAuthorsContinuesAfterBatchFailure(t *testing.T) {
	fake := &fakeWikidata{}
	source := NewWikidataSource(fake)

	// maxEntitiesPerRequest is 50, so 51 unique IDs force two batches: the
	// first one fails, the second one succeeds.
	candidates := make(map[string][]string, 51)
	for i := 0; i < 51; i++ {
		candidates[fmt.Sprintf("author-%d", i)] = []string{fmt.Sprintf("Q%d", i)}
	}

	results := make(map[string]model.Author)
	err := source.RetrieveAuthors(candidates, []string{"en"}, 0, func(slug string, a model.Author) error {
		results[slug] = a
		return nil
	})
	if err != nil {
		t.Fatalf("expected a batch failure not to abort the whole run, got error: %v", err)
	}
	if fake.batches != 2 {
		t.Fatalf("expected 2 batches to be attempted despite the first one failing, got %d", fake.batches)
	}
	if len(results) != len(candidates) {
		t.Fatalf("expected onResult to be called for all %d authors, got %d", len(candidates), len(results))
	}
}
