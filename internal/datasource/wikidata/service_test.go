package wikidata

import (
	"testing"

	gowikidata "github.com/Navid2zp/go-wikidata"
	"github.com/rickb777/date/v2"
)

func TestAuthor(t *testing.T) {
	for _, tcase := range testCases(t) {
		t.Run(tcase.name, func(t *testing.T) {
			mockSearchEntitiesRequest := SearchEntitiesRequestMock{
				GetFn: func() (*gowikidata.SearchEntitiesResponse, error) {
					return &tcase.pl.searchEntitiesResponse, nil
				},
			}

			mockGetEntitiesRequest := GetEntitiesRequestMock{
				SetPropsFn:     func(props []string) {},
				SetLanguagesFn: func(languages []string) {},
				GetFn: func() (*map[string]gowikidata.Entity, error) {
					return &map[string]gowikidata.Entity{
						"Q123456": tcase.pl.entity,
					}, nil
				},
			}

			mock := GowikidataMock{
				NewSearchFn: func(search string, language string) (SearchEntitiesRequest, error) {
					return mockSearchEntitiesRequest, nil
				},
				NewGetEntitiesFn: func(ids []string) (GetEntitiesRequest, error) {
					return mockGetEntitiesRequest, nil
				},
			}

			wikidataSource := NewWikidataSource(mock)

			author, err := wikidataSource.SearchAuthor("Miguel", "en")
			if err != nil {
				t.Errorf("Error retrieving author: %v", err)
			}
			if author.SourceID() != tcase.expectedValues.wikidataID {
				t.Errorf("Wrong source ID name, expected '%s', got '%s'", tcase.expectedValues.wikidataID, author.SourceID())
			}

			if author.Gender() != tcase.expectedValues.gender {
				t.Errorf("Wrong gender, expected '%d', got '%d'", tcase.expectedValues.gender, author.Gender())
			}

			if author.WikipediaLink("en") != tcase.expectedValues.wikipediaLink {
				t.Errorf("Wrong Wikipedia link, expected '%s', got '%s'", tcase.expectedValues.wikipediaLink, author.WikipediaLink("en"))
			}
		})
	}
}

type payloads struct {
	searchEntitiesResponse gowikidata.SearchEntitiesResponse
	entity                 gowikidata.Entity
}

type authorExpectedValues struct {
	wikidataID    string
	gender        int
	wikipediaLink string
	dateOfBirth   date.Date
}

type testCase struct {
	name           string
	pl             payloads
	expectedValues authorExpectedValues
}

func testCases(t *testing.T) []testCase {
	return []testCase{
		{
			name: "Author successfully retrieved",
			pl: struct {
				searchEntitiesResponse gowikidata.SearchEntitiesResponse
				entity                 gowikidata.Entity
			}{
				searchEntitiesResponse: searchEntitiesResponsePayloadBuilder("Q123456", "Miguel"),
				entity: entityPayloadBuilder(
					"Miguel",
					"Test description",
					"https://en.wikipedia.org/wiki/Miguel",
					map[string]string{
						propertySexOrGender: qidGenderMale,
						propertyDateOfBirth: "+1967-02-06T00:00:00Z",
					},
				),
			},
			expectedValues: authorExpectedValues{
				wikidataID:    "Q123456",
				gender:        GenderMale,
				wikipediaLink: "https://en.wikipedia.org/wiki/Miguel",
				dateOfBirth:   parseDate(t, "+1967-02-06T00:00:00Z"),
			},
		},
		{
			name: "Author not found",
			pl: struct {
				searchEntitiesResponse gowikidata.SearchEntitiesResponse
				entity                 gowikidata.Entity
			}{
				searchEntitiesResponse: searchEntitiesResponsePayloadBuilder("", ""),
				entity: entityPayloadBuilder(
					"",
					"",
					"",
					map[string]string{
						propertySexOrGender: "",
						propertyDateOfBirth: "",
					},
				),
			},
			expectedValues: authorExpectedValues{
				wikidataID:    "",
				gender:        GenderUnknown,
				wikipediaLink: "",
				dateOfBirth:   date.Zero,
			},
		},
	}
}

func searchEntitiesResponsePayloadBuilder(ID, title string) gowikidata.SearchEntitiesResponse {
	return gowikidata.SearchEntitiesResponse{
		SearchResult: []gowikidata.SearchEntity{
			{
				Title: title,
				ID:    ID,
			},
		},
	}
}

func entityPayloadBuilder(label, description, siteLink string, claims map[string]string) gowikidata.Entity {
	return gowikidata.Entity{
		Labels: map[string]gowikidata.Label{
			"en": {
				Value: label,
			},
		},
		Descriptions: map[string]gowikidata.Description{
			"en": {
				Value: description,
			},
		},
		SiteLinks: map[string]gowikidata.SiteLink{
			"enwiki": {
				URL: siteLink,
			},
		},
		Claims: map[string][]gowikidata.Claim{
			propertySexOrGender: {idClaimBuilder(claims[propertySexOrGender])},
			propertyDateOfBirth: {timeClaimBuilder(claims[propertyDateOfBirth])},
		},
	}
}

func idClaimBuilder(value string) gowikidata.Claim {
	return gowikidata.Claim{
		MainSnak: gowikidata.Snak{
			DataValue: gowikidata.DataValue{
				Value: gowikidata.DynamicDataValue{
					ValueFields: gowikidata.DataValueFields{
						ID: value,
					},
				},
			},
		},
	}
}

func timeClaimBuilder(value string) gowikidata.Claim {
	return gowikidata.Claim{
		MainSnak: gowikidata.Snak{
			DataValue: gowikidata.DataValue{
				Value: gowikidata.DynamicDataValue{
					ValueFields: gowikidata.DataValueFields{
						Time: value,
					},
				},
			},
		},
	}
}

func parseDate(t *testing.T, dateString string) date.Date {
	var parsed date.Date
	parsed, err := date.ParseISO(dateString)
	if err != nil {
		t.Fatalf("Error parsing date: %v", err)
	}
	return parsed
}
