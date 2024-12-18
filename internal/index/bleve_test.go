package index_test

import (
	"reflect"
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/metadata"
	"github.com/svera/coreander/v4/internal/result"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

func TestIndexAndSearch(t *testing.T) {
	for _, tcase := range testCases() {
		t.Run(tcase.name, func(t *testing.T) {
			indexMem, err := bleve.NewMemOnly(index.CreateMapping())
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

			appFS := afero.NewMemMapFs()
			idx := index.NewBleve(indexMem, appFS, "lib", mockMetadataReaders)

			// create test files and directories
			appFS.MkdirAll("lib", 0755)
			afero.WriteFile(appFS, tcase.filename, []byte(""), 0644)

			err = idx.AddLibrary(1, true)
			if err != nil {
				t.Errorf("Error indexing: %s", err.Error())
			}
			res, err := idx.Search(tcase.search, 1, 10)
			if err != nil {
				t.Errorf("Error searching: %s", err.Error())
			}
			if !reflect.DeepEqual(res, tcase.expectedResult) {
				t.Errorf("Wrong result returned, expected %#v, got %#v", tcase.expectedResult, res)
			}
		})
	}
}

type testCase struct {
	name           string
	filename       string
	mockedMeta     metadata.Metadata
	search         string
	expectedResult result.Paginated[[]index.Document]
}

func testCases() []testCase {
	return []testCase{
		{
			"Look for a term without accent must return accented results",
			"lib/book1.epub",
			metadata.Metadata{
				Title:       "Test A",
				Authors:     []string{"Pérez"},
				Description: "Just test metadata",
				Language:    "es",
				Subjects:    []string{"History", "Middle age"},
			},
			"perez",
			result.NewPaginated[[]index.Document](
				model.ResultsPerPage,
				1,
				1,
				[]index.Document{
					{
						ID:   "book1.epub",
						Slug: "perez-test-a",
						Metadata: metadata.Metadata{
							Title:       "Test A",
							Authors:     []string{"Pérez"},
							Description: "Just test metadata",
							Subjects:    []string{"History", "Middle age"},
						},
						AuthorsSlugs:  []string{"perez"},
						SeriesSlug:    "",
						SubjectsSlugs: []string{"history", "middle-age"},
					},
				},
			),
		},
		{
			"Look for a term without circumflex accent must return circumflexed results",
			"lib/book2.epub",
			metadata.Metadata{
				Title:       "Test B",
				Authors:     []string{"Benoît"},
				Description: "Just test metadata",
				Language:    "fr",
				Subjects:    []string{""},
			},
			"benoit",
			result.NewPaginated[[]index.Document](
				model.ResultsPerPage,
				1,
				1,
				[]index.Document{
					{
						ID:   "book2.epub",
						Slug: "benoit-test-b",
						Metadata: metadata.Metadata{
							Title:       "Test B",
							Authors:     []string{"Benoît"},
							Description: "Just test metadata",
							Subjects:    []string{""},
						},
						AuthorsSlugs:  []string{"benoit"},
						SeriesSlug:    "",
						SubjectsSlugs: []string{""},
					},
				},
			),
		},
		{
			"Look for several, not exact terms must return a result with all those terms, even if there is something in between",
			"lib/book3.epub",
			metadata.Metadata{
				Title:       "Test C",
				Authors:     []string{"Clifford D. Simak"},
				Description: "Just test metadata",
				Language:    "en",
				Subjects:    []string{""},
			},
			"clifford simak",
			result.NewPaginated[[]index.Document](
				model.ResultsPerPage,
				1,
				1,
				[]index.Document{
					{
						ID:   "book3.epub",
						Slug: "clifford-d-simak-test-c",
						Metadata: metadata.Metadata{
							Title:       "Test C",
							Authors:     []string{"Clifford D. Simak"},
							Description: "Just test metadata",
							Subjects:    []string{""},
						},
						AuthorsSlugs:  []string{"clifford-d-simak"},
						SeriesSlug:    "",
						SubjectsSlugs: []string{""},
					},
				},
			),
		},
		{
			"Look for several, not exact terms must return a result with all those terms, even if there is something in between",
			"lib/book4.epub",
			metadata.Metadata{
				Title:       "Test D",
				Authors:     []string{"James Ellroy"},
				Description: "Just test metadata",
				Language:    "en",
				Subjects:    []string{""},
			},
			"james ellroy",
			result.NewPaginated[[]index.Document](
				model.ResultsPerPage,
				1,
				1,
				[]index.Document{
					{
						ID:   "book4.epub",
						Slug: "james-ellroy-test-d",
						Metadata: metadata.Metadata{Title: "Test D",
							Authors:     []string{"James Ellroy"},
							Description: "Just test metadata",
							Subjects:    []string{""},
						},
						AuthorsSlugs:  []string{"james-ellroy"},
						SeriesSlug:    "",
						SubjectsSlugs: []string{""},
					},
				},
			),
		},
		{
			"Look for several, not exact terms with multiple leading, trailing and in-between spaces must return a result with all those terms, even if there is something in between",
			"lib/book5.epub",
			metadata.Metadata{
				Title:       "Test E",
				Authors:     []string{"James Ellroy"},
				Description: "Just test metadata",
				Language:    "en",
				Subjects:    []string{""},
			},
			" james       ellroy ",
			result.NewPaginated[[]index.Document](
				model.ResultsPerPage,
				1,
				1,
				[]index.Document{
					{
						ID:   "book5.epub",
						Slug: "james-ellroy-test-e",
						Metadata: metadata.Metadata{Title: "Test E",
							Authors:     []string{"James Ellroy"},
							Description: "Just test metadata",
							Subjects:    []string{""},
						},
						AuthorsSlugs:  []string{"james-ellroy"},
						SeriesSlug:    "",
						SubjectsSlugs: []string{""},
					},
				},
			),
		},
		{
			"Test genre spanish stemmer",
			"lib/book6.epub",
			metadata.Metadata{
				Title:       "La Guerrera",
				Authors:     []string{"Anónimo"},
				Description: "Just test metadata",
				Language:    "es",
				Subjects:    []string{"History", "Middle age"},
			},
			"guerrero",
			result.NewPaginated[[]index.Document](
				model.ResultsPerPage,
				1,
				1,
				[]index.Document{
					{
						ID:   "book6.epub",
						Slug: "anonimo-la-guerrera",
						Metadata: metadata.Metadata{
							Title:       "La Guerrera",
							Authors:     []string{"Anónimo"},
							Description: "Just test metadata",
							Subjects:    []string{"History", "Middle age"},
						},
						AuthorsSlugs:  []string{"anonimo"},
						SeriesSlug:    "",
						SubjectsSlugs: []string{"history", "middle-age"},
					},
				},
			),
		},
		{
			"Test plural italian stemmer",
			"lib/book7.epub",
			metadata.Metadata{
				Title:       "Fratelli",
				Authors:     []string{"Anónimo"},
				Description: "Just test metadata",
				Language:    "it",
				Subjects:    []string{"History", "Middle age"},
			},
			"fratello",
			result.NewPaginated[[]index.Document](
				model.ResultsPerPage,
				1,
				1,
				[]index.Document{
					{
						ID:   "book7.epub",
						Slug: "anonimo-fratelli",
						Metadata: metadata.Metadata{
							Title:       "Fratelli",
							Authors:     []string{"Anónimo"},
							Description: "Just test metadata",
							Subjects:    []string{"History", "Middle age"},
						},
						AuthorsSlugs:  []string{"anonimo"},
						SeriesSlug:    "",
						SubjectsSlugs: []string{"history", "middle-age"},
					},
				},
			),
		},
		{
			"Test genre spanish stemmer",
			"lib/book8.epub",
			metadata.Metadata{
				Title:       "El Infinito en un Junco",
				Authors:     []string{"Irene Vallejo"},
				Description: "Just test metadata",
				Language:    "es",
				Subjects:    []string{"History", "Middle age"},
			},
			"infinito junco",
			result.NewPaginated[[]index.Document](
				model.ResultsPerPage,
				1,
				1,
				[]index.Document{
					{
						ID:   "book8.epub",
						Slug: "irene-vallejo-el-infinito-en-un-junco",
						Metadata: metadata.Metadata{
							Title:       "El Infinito en un Junco",
							Authors:     []string{"Irene Vallejo"},
							Description: "Just test metadata",
							Subjects:    []string{"History", "Middle age"},
						},
						AuthorsSlugs:  []string{"irene-vallejo"},
						SeriesSlug:    "",
						SubjectsSlugs: []string{"history", "middle-age"},
					},
				},
			),
		},
		{
			"Test spanish stemmer returning accented word while using unaccented word in search",
			"lib/book9.epub",
			metadata.Metadata{
				Title:       "Últimos días en Colditz",
				Authors:     []string{"Patrick R. Reid"},
				Description: "Just test metadata",
				Language:    "es",
				Subjects:    []string{"History", "WWII"},
			},
			"ultimos",
			result.NewPaginated[[]index.Document](
				model.ResultsPerPage,
				1,
				1,
				[]index.Document{
					{
						ID:   "book9.epub",
						Slug: "patrick-r-reid-ultimos-dias-en-colditz",
						Metadata: metadata.Metadata{
							Title:       "Últimos días en Colditz",
							Authors:     []string{"Patrick R. Reid"},
							Description: "Just test metadata",
							Subjects:    []string{"History", "WWII"},
						},
						AuthorsSlugs:  []string{"patrick-r-reid"},
						SeriesSlug:    "",
						SubjectsSlugs: []string{"history", "wwii"},
					},
				},
			),
		},
	}
}
