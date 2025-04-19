package wikidata

import (
	"reflect"
	"testing"

	gowikidata "github.com/Navid2zp/go-wikidata"
	"github.com/svera/coreander/v4/internal/precisiondate"
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
