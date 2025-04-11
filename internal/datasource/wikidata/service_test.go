package wikidata

import (
	"testing"

	gowikidata "github.com/Navid2zp/go-wikidata"
	"github.com/rickb777/date/v2"
)

func TestAuthor(t *testing.T) {
	mockServer := NewMockServer(t, "fixtures")
	defer mockServer.Close()
	gowikidata.WikidataDomain = mockServer.URL

	for _, tcase := range testCases(t) {
		t.Run(tcase.name, func(t *testing.T) {

			wikidataSource := NewWikidataSource(Gowikidata{})

			author, err := wikidataSource.SearchAuthor(tcase.search, []string{"en"})
			if err != nil && tcase.expectedValues != nil {
				t.Errorf("Error retrieving author: %v", err)
			}
			if tcase.expectedValues == nil {
				return
			}

			if author.SourceID() != tcase.expectedValues.wikidataID {
				t.Errorf("Wrong source ID name, expected '%s', got '%s'", tcase.expectedValues.wikidataID, author.SourceID())
			}

			if author.Gender() != tcase.expectedValues.gender {
				t.Errorf("Wrong gender, expected '%f', got '%f'", tcase.expectedValues.gender, author.Gender())
			}

			if expected, actual := tcase.expectedValues.website, author.Website(); author.Website() != tcase.expectedValues.website {
				t.Errorf("Wrong website link, expected '%s', got '%s'", expected, actual)
			}
		})
	}
}

type authorExpectedValues struct {
	wikidataID  string
	gender      float64
	website     string
	dateOfBirth date.Date
}

type testCase struct {
	name           string
	search         string
	expectedValues *authorExpectedValues
}

func testCases(t *testing.T) []testCase {
	return []testCase{
		{
			name:   "Author successfully retrieved",
			search: "Miguel",
			expectedValues: &authorExpectedValues{
				wikidataID:  "Q1234",
				gender:      GenderMale,
				website:     "https://douglasadams.com",
				dateOfBirth: parseISODate(t, "+1967-02-06T00:00:00Z"),
			},
		},
		{
			name:           "Author not found",
			search:         "Eufrasio",
			expectedValues: nil,
		},
		{
			name:           "Found entry is not human",
			search:         "Q1234",
			expectedValues: nil,
		},
	}
}

func parseISODate(t *testing.T, dateString string) date.Date {
	var parsed date.Date
	parsed, err := date.ParseISO(dateString)
	if err != nil {
		t.Fatalf("error parsing date: %v", err)
	}
	return parsed
}
