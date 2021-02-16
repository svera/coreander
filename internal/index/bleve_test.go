package index_test

import (
	"reflect"
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/internal/index"
	"github.com/svera/coreander/internal/metadata"
)

func TestIndexAndSearch(t *testing.T) {
	for _, tcase := range testCases() {
		t.Run(tcase.name, func(t *testing.T) {
			indexMapping := bleve.NewIndexMapping()
			index.AddMappings(indexMapping)
			indexMem, err := bleve.NewMemOnly(indexMapping)
			if err != nil {
				t.Errorf("Error initialising index")
			}

			mockMetadataReaders := map[string]metadata.Reader{
				".epub": metadata.ReaderMock{
					MetadataFake: func(file string) (metadata.Metadata, error) {
						return tcase.mockedMeta, nil
					},
				},
			}

			idx := index.NewBleve(indexMem, "lib", mockMetadataReaders)

			appFS := afero.NewMemMapFs()
			// create test files and directories
			appFS.MkdirAll("lib", 0755)
			afero.WriteFile(appFS, tcase.filename, []byte(""), 0644)

			err = idx.AddLibrary(appFS, 1)
			if err != nil {
				t.Errorf("Error indexing: %s", err.Error())
			}
			res, err := idx.Search(tcase.search, 1, 10)
			if err != nil {
				t.Errorf("Error searching: %s", err.Error())
			}
			if !reflect.DeepEqual(*res, tcase.expectedResult) {
				t.Errorf("Wrong result returned, expected %v, got %v", tcase.expectedResult, res)
			}
		})
	}
}

type testCase struct {
	name           string
	filename       string
	mockedMeta     metadata.Metadata
	search         string
	expectedResult index.Result
}

func testCases() []testCase {
	return []testCase{
		{
			"Look for a term without accent must return accented results",
			"lib/book1.epub",
			metadata.Metadata{
				Title:       "Test A",
				Author:      "Pérez",
				Description: "Just test metadata",
				Language:    "es",
			},
			"perez",
			index.Result{
				Page:       1,
				TotalPages: 1,
				TotalHits:  1,
				Hits: map[string]metadata.Metadata{
					"book1.epub": {
						Title:       "Test A",
						Author:      "Pérez",
						Description: "Just test metadata",
					},
				},
			},
		},
		{
			"Look for a term without circumflex accent must return circumflexed results",
			"lib/book2.epub",
			metadata.Metadata{
				Title:       "Test B",
				Author:      "Benoît",
				Description: "Just test metadata",
				Language:    "fr",
			},
			"benoit",
			index.Result{
				Page:       1,
				TotalPages: 1,
				TotalHits:  1,
				Hits: map[string]metadata.Metadata{
					"book2.epub": {
						Title:       "Test B",
						Author:      "Benoît",
						Description: "Just test metadata",
					},
				},
			},
		},
		{
			"Look for a several not exact terms must return a result with all those terms, even if there is something in between",
			"lib/book3.epub",
			metadata.Metadata{
				Title:       "Test C",
				Author:      "Clifford D. Simak",
				Description: "Just test metadata",
				Language:    "en",
			},
			"clifford simak",
			index.Result{
				Page:       1,
				TotalPages: 1,
				TotalHits:  1,
				Hits: map[string]metadata.Metadata{
					"book3.epub": {
						Title:       "Test C",
						Author:      "Clifford D. Simak",
						Description: "Just test metadata",
					},
				},
			},
		},
		{
			"Look for a several not exact terms must return a result with all those terms, even if there is something in between",
			"lib/book4.epub",
			metadata.Metadata{
				Title:       "Test D",
				Author:      "James Ellroy",
				Description: "Just test metadata",
				Language:    "en",
			},
			"james ellroy",
			index.Result{
				Page:       1,
				TotalPages: 1,
				TotalHits:  1,
				Hits: map[string]metadata.Metadata{
					"book4.epub": {
						Title:       "Test D",
						Author:      "James Ellroy",
						Description: "Just test metadata",
					},
				},
			},
		},
	}
}
