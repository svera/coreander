package index_test

import (
	"reflect"
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/pirmd/epub"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/metadata"
	"github.com/svera/coreander/v4/internal/precisiondate"
	"github.com/svera/coreander/v4/internal/result"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

func TestIndexAndSearch(t *testing.T) {
	for _, tcase := range testIndexAndSearchCases() {
		t.Run(tcase.name, func(t *testing.T) {
			indexMem, err := bleve.NewMemOnly(index.CreateMapping())
			if err != nil {
				t.Errorf("Error initialising index")
			}

			mockMetadataReaders := map[string]metadata.Reader{
				".epub": metadata.EpubReader{
					GetMetadataFromFile: func(file string) (*epub.Information, error) {
						return tcase.mockedMeta, nil
					},
					GetPackageFromFile: epub.GetPackageFromFile,
				},
			}

			appFS := afero.NewMemMapFs()
			// create test files and directories
			appFS.MkdirAll("lib", 0755)
			if err = afero.WriteFile(appFS, tcase.filename, []byte(""), 0644); err != nil {
				t.Errorf("Couldn't write file %s", tcase.filename)
			}

			idx := index.NewBleve(indexMem, appFS, "lib", mockMetadataReaders)

			if err = idx.AddLibrary(1, true); err != nil {
				t.Errorf("Error indexing: %s", err.Error())
			}
			res, err := idx.Search(tcase.search, 1, 10)
			if err != nil {
				t.Errorf("Error searching: %s", err.Error())
			}
			if !reflect.DeepEqual(res, tcase.expectedResult) {
				t.Errorf("Wrong result returned, expected\n %#v,\n got\n %#v\n", tcase.expectedResult, res)
			}
		})
	}
}

type testCase struct {
	name           string
	filename       string
	mockedMeta     *epub.Information
	search         index.SearchFields
	expectedResult result.Paginated[[]index.Document]
}

func testIndexAndSearchCases() []testCase {
	return []testCase{
		{
			"Look for a term without accent must return accented results",
			"lib/book1.epub",
			&epub.Information{
				Title: []string{"Test A"},
				Creator: []epub.Author{
					{
						FullName: "Pérez",
						Role:     "aut",
					},
				},
				Description: []string{"Just test metadata"},
				Language:    []string{"es"},
				Subject:     []string{"History", "Middle age"},
				Date: []epub.Date{
					{
						Stamp: "2023-10-01",
						Event: "publication",
					},
				},
			},
			index.SearchFields{Keywords: "perez"},
			result.NewPaginated(
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
							Description: "<p>Just test metadata</p>",
							Subjects:    []string{"History", "Middle age"},
							Format:      "EPUB",
							Publication: precisiondate.NewPrecisionDate("2023-10-01T00:00:00Z", precisiondate.PrecisionDay),
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
			&epub.Information{
				Title: []string{"Test B"},
				Creator: []epub.Author{
					{
						FullName: "Benoît",
						Role:     "aut",
					},
				},
				Description: []string{"Just test metadata"},
				Language:    []string{"fr"},
				Subject:     []string{},
				Date: []epub.Date{
					{
						Stamp: "1974",
						Event: "publication",
					},
				},
			},
			index.SearchFields{Keywords: "benoit"},
			result.NewPaginated(
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
							Description: "<p>Just test metadata</p>",
							Subjects:    []string{},
							Format:      "EPUB",
							Publication: precisiondate.NewPrecisionDate("1974-01-01T00:00:00Z", precisiondate.PrecisionYear),
						},
						AuthorsSlugs:  []string{"benoit"},
						SeriesSlug:    "",
						SubjectsSlugs: []string{},
					},
				},
			),
		},
		{
			"Look for several, not exact terms must return a result with all those terms, even if there is something in between",
			"lib/book3.epub",
			&epub.Information{
				Title: []string{"Test C"},
				Creator: []epub.Author{
					{
						FullName: "Clifford D. Simak",
						Role:     "aut",
					},
				},
				Description: []string{"Just test metadata"},
				Language:    []string{"en"},
				Subject:     []string{},
				Date: []epub.Date{
					{
						Stamp: "1950-11-02T00:00:00Z",
						Event: "publication",
					},
				},
			},
			index.SearchFields{Keywords: "clifford simak"},
			result.NewPaginated(
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
							Description: "<p>Just test metadata</p>",
							Subjects:    []string{},
							Format:      "EPUB",
							Publication: precisiondate.NewPrecisionDate("1950-11-02T00:00:00Z", precisiondate.PrecisionDay),
						},
						AuthorsSlugs:  []string{"clifford-d-simak"},
						SeriesSlug:    "",
						SubjectsSlugs: []string{},
					},
				},
			),
		},
		{
			"Look for several, not exact terms must return a result with all those terms, even if there is something in between",
			"lib/book4.epub",
			&epub.Information{
				Title: []string{"Test D"},
				Creator: []epub.Author{
					{
						FullName: "James Ellroy",
						Role:     "aut",
					},
				},
				Description: []string{"Just test metadata"},
				Language:    []string{"en"},
				Subject:     []string{},
			},
			index.SearchFields{Keywords: "james ellroy"},
			result.NewPaginated(
				model.ResultsPerPage,
				1,
				1,
				[]index.Document{
					{
						ID:   "book4.epub",
						Slug: "james-ellroy-test-d",
						Metadata: metadata.Metadata{Title: "Test D",
							Authors:     []string{"James Ellroy"},
							Description: "<p>Just test metadata</p>",
							Subjects:    []string{},
							Format:      "EPUB",
							Publication: precisiondate.PrecisionDate{Precision: precisiondate.PrecisionDay},
						},
						AuthorsSlugs:  []string{"james-ellroy"},
						SeriesSlug:    "",
						SubjectsSlugs: []string{},
					},
				},
			),
		},
		{
			"Look for several, not exact terms with multiple leading, trailing and in-between spaces must return a result with all those terms, even if there is something in between",
			"lib/book5.epub",
			&epub.Information{
				Title: []string{"Test E"},
				Creator: []epub.Author{
					{
						FullName: "James Ellroy",
						Role:     "aut",
					},
				},
				Description: []string{"Just test metadata"},
				Language:    []string{"en"},
				Subject:     []string{},
			},
			index.SearchFields{Keywords: " james       ellroy "},
			result.NewPaginated(
				model.ResultsPerPage,
				1,
				1,
				[]index.Document{
					{
						ID:   "book5.epub",
						Slug: "james-ellroy-test-e",
						Metadata: metadata.Metadata{Title: "Test E",
							Authors:     []string{"James Ellroy"},
							Description: "<p>Just test metadata</p>",
							Subjects:    []string{},
							Format:      "EPUB",
							Publication: precisiondate.PrecisionDate{Precision: precisiondate.PrecisionDay},
						},
						AuthorsSlugs:  []string{"james-ellroy"},
						SeriesSlug:    "",
						SubjectsSlugs: []string{},
					},
				},
			),
		},
		{
			"Test genre spanish stemmer",
			"lib/book6.epub",
			&epub.Information{
				Title: []string{"La Guerrera"},
				Creator: []epub.Author{
					{
						FullName: "Anónimo",
						Role:     "aut",
					},
				},
				Description: []string{"Just test metadata"},
				Language:    []string{"es"},
				Subject:     []string{"History", "Middle age"},
			},
			index.SearchFields{Keywords: "guerrero"},
			result.NewPaginated(
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
							Description: "<p>Just test metadata</p>",
							Subjects:    []string{"History", "Middle age"},
							Format:      "EPUB",
							Publication: precisiondate.PrecisionDate{Precision: precisiondate.PrecisionDay},
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
			&epub.Information{
				Title: []string{"Fratelli"},
				Creator: []epub.Author{
					{
						FullName: "Anónimo",
						Role:     "aut",
					},
				},
				Description: []string{"Just test metadata"},
				Language:    []string{"it"},
				Subject:     []string{"History", "Middle age"},
			},
			index.SearchFields{Keywords: "fratello"},
			result.NewPaginated(
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
							Description: "<p>Just test metadata</p>",
							Subjects:    []string{"History", "Middle age"},
							Format:      "EPUB",
							Publication: precisiondate.PrecisionDate{Precision: precisiondate.PrecisionDay},
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
			&epub.Information{
				Title: []string{"El Infinito en un Junco"},
				Creator: []epub.Author{
					{
						FullName: "Irene Vallejo",
						Role:     "aut",
					},
				},
				Description: []string{"Just test metadata"},
				Language:    []string{"es"},
				Subject:     []string{"History", "Middle age"},
			},
			index.SearchFields{Keywords: "infinito junco"},
			result.NewPaginated(
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
							Description: "<p>Just test metadata</p>",
							Subjects:    []string{"History", "Middle age"},
							Format:      "EPUB",
							Publication: precisiondate.PrecisionDate{Precision: precisiondate.PrecisionDay},
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
			&epub.Information{
				Title: []string{"Últimos días en Colditz"},
				Creator: []epub.Author{
					{
						FullName: "Patrick R. Reid",
						Role:     "aut",
					},
				},
				Description: []string{"Just test metadata"},
				Language:    []string{"es"},
				Subject:     []string{"History", "WWII"},
			},
			index.SearchFields{Keywords: "ultimos"},
			result.NewPaginated(
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
							Description: "<p>Just test metadata</p>",
							Subjects:    []string{"History", "WWII"},
							Format:      "EPUB",
							Publication: precisiondate.PrecisionDate{Precision: precisiondate.PrecisionDay},
						},
						AuthorsSlugs:  []string{"patrick-r-reid"},
						SeriesSlug:    "",
						SubjectsSlugs: []string{"history", "wwii"},
					},
				},
			),
		},
		{
			"Weird case with ',' as subject or '&' as author",
			"lib/book10.epub",
			&epub.Information{
				Title: []string{"Sin nombre"},
				Creator: []epub.Author{
					{
						FullName: "&",
						Role:     "aut",
					},
				},
				Description: []string{"Just test metadata"},
				Language:    []string{"es"},
				Subject:     []string{","},
			},
			index.SearchFields{Keywords: "sin nombre"},
			result.NewPaginated(
				model.ResultsPerPage,
				1,
				1,
				[]index.Document{
					{
						ID:   "book10.epub",
						Slug: "sin-nombre",
						Metadata: metadata.Metadata{
							Title:       "Sin nombre",
							Authors:     []string{""},
							Description: "<p>Just test metadata</p>",
							Subjects:    []string{},
							Format:      "EPUB",
							Publication: precisiondate.PrecisionDate{Precision: precisiondate.PrecisionDay},
						},
						AuthorsSlugs:  []string{""},
						SeriesSlug:    "",
						SubjectsSlugs: []string{},
					},
				},
			),
		},
		{
			"Test search with partial title and author",
			"lib/book8.epub",
			&epub.Information{
				Title: []string{"El Infinito en un Junco"},
				Creator: []epub.Author{
					{
						FullName: "Irene Vallejo",
						Role:     "aut",
					},
				},
				Description: []string{"Just test metadata"},
				Language:    []string{"es"},
				Subject:     []string{"History", "Middle age"},
			},
			index.SearchFields{Keywords: "irene junco"},
			result.NewPaginated(
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
							Description: "<p>Just test metadata</p>",
							Subjects:    []string{"History", "Middle age"},
							Format:      "EPUB",
							Publication: precisiondate.PrecisionDate{Precision: precisiondate.PrecisionDay},
						},
						AuthorsSlugs:  []string{"irene-vallejo"},
						SeriesSlug:    "",
						SubjectsSlugs: []string{"history", "middle-age"},
					},
				},
			),
		},
	}
}
