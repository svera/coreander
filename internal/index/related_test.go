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
)

func TestSameSubjects(t *testing.T) {
	indexMem, err := bleve.NewMemOnly(index.CreateMapping())
	if err != nil {
		t.Errorf("Error instancing Bleve: %s", err.Error())
	}
	library := mockedLibrary()

	mockMetadataReaders := map[string]metadata.Reader{
		".epub": metadata.EpubReader{
			GetMetadataFromFile: func(file string) (*epub.Information, error) {
				return library[file], nil
			},
			GetPackageFromFile: epub.GetPackageFromFile,
		},
	}

	appFS := afero.NewMemMapFs()
	// create test files and directories
	appFS.MkdirAll("lib", 0755)
	for filename := range library {
		if err = afero.WriteFile(appFS, filename, []byte(""), 0644); err != nil {
			t.Errorf("Couldn't write file %s", filename)
		}
	}

	idx := index.NewBleve(indexMem, appFS, "lib", mockMetadataReaders)

	if err = idx.AddLibrary(1, true); err != nil {
		t.Errorf("Error indexing: %s", err.Error())
	}

	for _, tcase := range testSameSubjectsCases() {
		t.Run(tcase.name, func(t *testing.T) {
			if err != nil {
				t.Errorf("Error initialising index")
			}

			res, err := idx.SameSubjects(tcase.slug, 4)
			if err != nil {
				t.Errorf("Error searching: %s", err.Error())
			}
			if !reflect.DeepEqual(res, tcase.expectedResult) {
				t.Errorf("Wrong result returned for %s, expected\n %#v,\n got\n %#v\n", tcase.slug, tcase.expectedResult, res)
			}
		})
	}
}

type sameSubjectsTestCase struct {
	name           string
	slug           string
	expectedResult []index.Document
}

func mockedLibrary() map[string]*epub.Information {
	return map[string]*epub.Information{
		"lib/file1.epub": {
			Title: []string{"Test A"},
			Creator: []epub.Author{
				{
					FullName: "Pedro Pérez",
					Role:     "aut",
				},
			},
			Description: []string{"Just test metadata"},
			Language:    []string{"en"},
			Subject:     []string{"History", "Middle age"},
			Date: []epub.Date{
				{
					Stamp: "2010-10-01",
					Event: "publication",
				},
			},
		},
		"lib/file2.epub": {
			Title: []string{"Test B"},
			Creator: []epub.Author{
				{
					FullName: "John Thompson",
					Role:     "aut",
				},
			},
			Description: []string{"Just test metadata"},
			Language:    []string{"en"},
			Subject:     []string{"History", "Middle age"},
			Date: []epub.Date{
				{
					Stamp: "2014-03-05",
					Event: "publication",
				},
			},
		},
		"lib/file3.epub": {
			Title: []string{"Test C"},
			Creator: []epub.Author{
				{
					FullName: "Isaac Asimov",
					Role:     "aut",
				},
			},
			Description: []string{"Just test metadata"},
			Language:    []string{"en"},
			Subject:     []string{"History", "Middle age"},
			Date: []epub.Date{
				{
					Stamp: "2011-05-14",
					Event: "publication",
				},
			},
		},
		"lib/file4.epub": {
			Title: []string{"Test D"},
			Creator: []epub.Author{
				{
					FullName: "Alexandre Dumas",
					Role:     "aut",
				},
			},
			Description: []string{"Just test metadata"},
			Language:    []string{"en"},
			Subject:     []string{"Novel", "Adventures"},
			Date: []epub.Date{
				{
					Stamp: "1845-05-14",
					Event: "publication",
				},
			},
		},
		"lib/file5.epub": {
			Title: []string{"Test E"},
			Creator: []epub.Author{
				{
					FullName: "Giacomo Leopardi",
					Role:     "aut",
				},
			},
			Description: []string{"Just test metadata"},
			Language:    []string{"en"},
			Subject:     []string{"History"},
			Date: []epub.Date{
				{
					Stamp: "2010-11-05",
					Event: "publication",
				},
			},
		},
	}
}

func testSameSubjectsCases() []sameSubjectsTestCase {
	return []sameSubjectsTestCase{
		{
			"Get documents with same subjects sorted by temporal proximity to the reference document",
			"isaac-asimov-test-c",
			[]index.Document{
				{
					ID:   "file1.epub",
					Slug: "pedro-perez-test-a",
					Metadata: metadata.Metadata{
						Title:       "Test A",
						Authors:     []string{"Pedro Pérez"},
						Description: "<p>Just test metadata</p>",
						Subjects:    []string{"History", "Middle age"},
						Format:      "EPUB",
						Publication: precisiondate.NewPrecisionDate("2010-10-01T00:00:00Z", precisiondate.PrecisionDay),
					},
					AuthorsSlugs:  []string{"pedro-perez"},
					SeriesSlug:    "",
					SubjectsSlugs: []string{"history", "middle-age"},
				},
				{
					ID:   "file2.epub",
					Slug: "john-thompson-test-b",
					Metadata: metadata.Metadata{
						Title:       "Test B",
						Authors:     []string{"John Thompson"},
						Description: "<p>Just test metadata</p>",
						Subjects:    []string{"History", "Middle age"},
						Format:      "EPUB",
						Publication: precisiondate.NewPrecisionDate("2014-03-05T00:00:00Z", precisiondate.PrecisionDay),
					},
					AuthorsSlugs:  []string{"john-thompson"},
					SeriesSlug:    "",
					SubjectsSlugs: []string{"history", "middle-age"},
				},
				{
					ID:   "file5.epub",
					Slug: "giacomo-leopardi-test-e",
					Metadata: metadata.Metadata{
						Title:       "Test E",
						Authors:     []string{"Giacomo Leopardi"},
						Description: "<p>Just test metadata</p>",
						Subjects:    []string{"History"},
						Format:      "EPUB",
						Publication: precisiondate.NewPrecisionDate("2010-11-05T00:00:00Z", precisiondate.PrecisionDay),
					},
					AuthorsSlugs:  []string{"giacomo-leopardi"},
					SeriesSlug:    "",
					SubjectsSlugs: []string{"history"},
				},
			},
		},
	}
}
