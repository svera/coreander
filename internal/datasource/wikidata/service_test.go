package wikidata_test

import (
	"testing"

	gowikidata "github.com/Navid2zp/go-wikidata"
	"github.com/svera/coreander/v4/internal/datasource/wikidata"
	"github.com/svera/coreander/v4/internal/index"
)

func TestAuthor(t *testing.T) {
	for _, tcase := range testCases() {
		t.Run(tcase.name, func(t *testing.T) {
			mockSearchEntitiesRequest := wikidata.SearchEntitiesRequestMock{
				GetFn: func() (*gowikidata.SearchEntitiesResponse, error) {
					return &tcase.wikidata.searchEntitiesResponse, nil
				},
			}

			mockGetEntitiesRequest := wikidata.GetEntitiesRequestMock{
				SetPropsFn:     func(props []string) {},
				SetLanguagesFn: func(languages []string) {},
				GetFn: func() (*map[string]gowikidata.Entity, error) {
					return &map[string]gowikidata.Entity{
						"Q123456": tcase.wikidata.entity,
					}, nil
				},
			}

			mock := wikidata.GowikidataMock{
				NewSearchFn: func(search string, language string) (wikidata.SearchEntitiesRequest, error) {
					return mockSearchEntitiesRequest, nil
				},
				NewGetEntitiesFn: func(ids []string) (wikidata.GetEntitiesRequest, error) {
					return mockGetEntitiesRequest, nil
				},
			}

			wikidataSource := wikidata.NewWikidataSource(mock)

			indexAuthor := index.Author{
				Name: "Test Author",
			}
			author, err := wikidataSource.Author(indexAuthor, "en")
			if err != nil {
				t.Errorf("Error retrieving author: %v", err)
			}
			if author.SourceID() != tcase.authorExpectedValues.wikidataID {
				t.Errorf("Wrong author name, expected 'Test Author', got '%s'", author.SourceID())
			}
		})
	}
}

type testCase struct {
	name     string
	wikidata struct {
		searchEntitiesResponse gowikidata.SearchEntitiesResponse
		entity                 gowikidata.Entity
	}
	authorExpectedValues struct {
		wikidataID string
	}
}

func testCases() []testCase {
	return []testCase{
		{
			name: "Successful retrieval",
			wikidata: struct {
				searchEntitiesResponse gowikidata.SearchEntitiesResponse
				entity                 gowikidata.Entity
			}{
				searchEntitiesResponse: gowikidata.SearchEntitiesResponse{
					SearchResult: []gowikidata.SearchEntity{
						{
							Title: "Test Author",
							ID:    "Q123456",
						},
					},
				},
				entity: gowikidata.Entity{
					Labels: map[string]gowikidata.Label{
						"en": {
							Value: "Test Author",
						},
					},
					Descriptions: map[string]gowikidata.Description{
						"en": {
							Value: "Test description",
						},
					},
					SiteLinks: map[string]gowikidata.SiteLink{
						"enwiki": {
							URL: "https://en.wikipedia.org/wiki/Test_Author",
						},
					},
				},
			},
			authorExpectedValues: struct{ wikidataID string }{
				wikidataID: "Q123456",
			},
		},
	}
}
