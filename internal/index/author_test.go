package index_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/pirmd/epub"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/metadata"
	"github.com/svera/coreander/v4/internal/precisiondate"
)

func TestAge(t *testing.T) {
	for _, tcase := range testCasesAuthorAge() {
		t.Run(tcase.name, func(t *testing.T) {
			if age, expectedAge := tcase.author.Age(), tcase.expectedAge; age != expectedAge {
				t.Errorf("Wrong author age, expected '%d', got '%d'", expectedAge, age)
			}
		})
	}
}

type testCaseAuthorAge struct {
	name        string
	author      index.Author
	expectedAge int
}

func testCasesAuthorAge() []testCaseAuthorAge {
	return []testCaseAuthorAge{
		{
			name: "Leigh Brackett",
			author: index.Author{
				DateOfBirth: precisiondate.NewPrecisionDate(
					"+1915-12-07T00:00:00Z",
					precisiondate.PrecisionDay,
				),
				DateOfDeath: precisiondate.NewPrecisionDate(
					"+1978-03-18T00:00:00Z",
					precisiondate.PrecisionDay,
				),
			},
			expectedAge: 62,
		},
		{
			name: "Juan Luis Arinaga",
			author: index.Author{
				DateOfBirth: precisiondate.NewPrecisionDate(
					"+1954-09-28T00:00:00Z",
					precisiondate.PrecisionDay,
				),
				DateOfDeath: precisiondate.NewPrecisionDate(
					"+2025-09-19T00:00:00Z",
					precisiondate.PrecisionDay,
				),
			},
			expectedAge: 70,
		},
		{
			name: "Lucius Annaeus Seneca (not enough precision in date of birth)",
			author: index.Author{
				DateOfBirth: precisiondate.NewPrecisionDate(
					"-0004-00-00T00:00:00Z",
					precisiondate.PrecisionYear,
				),
				DateOfDeath: precisiondate.NewPrecisionDate(
					"+0065-04-12T00:00:00Z",
					precisiondate.PrecisionDay,
				),
			},
			expectedAge: 0,
		},
	}
}

func TestBirthNameIncludesName(t *testing.T) {
	for _, tcase := range testCasesAuthorBirthNameIncludesName() {
		t.Run(tcase.name, func(t *testing.T) {
			if result, expectedResult := tcase.author.BirthNameIncludesName(), tcase.expectedResult; result != expectedResult {
				t.Errorf("Wrong result in birth name includes name, expected '%v', got '%v'", result, expectedResult)
			}
		})
	}
}

type testCaseAuthorBirthNameIncludesName struct {
	name           string
	author         index.Author
	expectedResult bool
}

func testCasesAuthorBirthNameIncludesName() []testCaseAuthorBirthNameIncludesName {
	return []testCaseAuthorBirthNameIncludesName{
		{
			name: "Arturo Pérez-Reverte Gutiérrez",
			author: index.Author{
				Name:      "Arturo Pérez-Reverte",
				BirthName: "Arturo Pérez-Reverte Gutiérrez",
			},
			expectedResult: true,
		},
		{
			name: "George Orwell",
			author: index.Author{
				Name:      "George Orwell",
				BirthName: "Eric Arthur Blair",
			},
			expectedResult: false,
		},
	}
}

func TestAuthorSubjectsMatchDocuments(t *testing.T) {
	indexMem, err := bleve.NewMemOnly(index.CreateDocumentsMapping())
	if err != nil {
		t.Fatalf("Error initialising index: %v", err)
	}

	authorSlug := "test-author"
	authorName := "Test Author"

	mockMetadataReaders := map[string]metadata.Reader{
		".epub": metadata.EpubReader{
			GetMetadataFromFile: func(file string) (*epub.Information, error) {
				switch file {
				case "lib/book1.epub":
					return &epub.Information{
						Title: []string{"Book 1"},
						Creator: []epub.Author{
							{FullName: authorName, Role: "aut"},
						},
						Description: []string{"First book"},
						Language:    []string{"en"},
						Subject:     []string{"Fiction", "Mystery"},
						Date: []epub.Date{
							{Stamp: "2020-01-01", Event: "publication"},
						},
					}, nil
				case "lib/book2.epub":
					return &epub.Information{
						Title: []string{"Book 2"},
						Creator: []epub.Author{
							{FullName: authorName, Role: "aut"},
						},
						Description: []string{"Second book"},
						Language:    []string{"en"},
						Subject:     []string{"Fiction", "Science Fiction"},
						Date: []epub.Date{
							{Stamp: "2021-01-01", Event: "publication"},
						},
					}, nil
				case "lib/book3.epub":
					return &epub.Information{
						Title: []string{"Book 3"},
						Creator: []epub.Author{
							{FullName: authorName, Role: "aut"},
						},
						Description: []string{"Third book"},
						Language:    []string{"en"},
						Subject:     []string{"Non-Fiction", "History"},
						Date: []epub.Date{
							{Stamp: "2022-01-01", Event: "publication"},
						},
					}, nil
				default:
					return nil, fmt.Errorf("unknown file: %s", file)
				}
			},
			GetPackageFromFile: epub.GetPackageFromFile,
		},
	}

	appFS := afero.NewMemMapFs()
	appFS.MkdirAll("lib", 0755)

	// Create test files
	testFiles := []string{"lib/book1.epub", "lib/book2.epub", "lib/book3.epub"}
	for _, file := range testFiles {
		if err = afero.WriteFile(appFS, file, []byte(""), 0644); err != nil {
			t.Fatalf("Couldn't write file %s: %v", file, err)
		}
	}

	authorsIndexMem, _ := bleve.NewMemOnly(index.CreateAuthorsMapping())
	idx := index.NewBleve(indexMem, authorsIndexMem, appFS, "lib", mockMetadataReaders)

	if err = idx.AddLibrary(1, true); err != nil {
		t.Fatalf("Error indexing: %v", err)
	}

	// Get the author
	author, err := idx.Author(authorSlug, "")
	if err != nil {
		t.Fatalf("Error getting author: %v", err)
	}
	if author.Slug == "" {
		t.Fatalf("Author not found")
	}

	// Get all documents by this author
	documents, err := idx.SearchByAuthor(index.SearchFields{Keywords: authorSlug}, 1, 100)
	if err != nil {
		t.Fatalf("Error searching documents by author: %v", err)
	}

	// Collect all unique subject slugs from documents
	expectedSubjectsSlugsMap := make(map[string]struct{})

	for _, doc := range documents.Hits() {
		for _, slug := range doc.SubjectsSlugs {
			if slug != "" {
				expectedSubjectsSlugsMap[slug] = struct{}{}
			}
		}
	}

	// Convert map to sorted slice
	expectedSubjectsSlugs := make([]string, 0, len(expectedSubjectsSlugsMap))
	for slug := range expectedSubjectsSlugsMap {
		expectedSubjectsSlugs = append(expectedSubjectsSlugs, slug)
	}
	slices.Sort(expectedSubjectsSlugs)

	// Sort author's subject slugs for comparison
	authorSubjectsSlugs := make([]string, len(author.SubjectsSlugs))
	copy(authorSubjectsSlugs, author.SubjectsSlugs)
	slices.Sort(authorSubjectsSlugs)

	// Verify subject slugs match (these use keyword mapping and aren't analyzed)
	if !slices.Equal(authorSubjectsSlugs, expectedSubjectsSlugs) {
		t.Errorf("Author subject slugs do not match document subject slugs.\nExpected: %v\nGot: %v", expectedSubjectsSlugs, authorSubjectsSlugs)
	}

	if len(authorSubjectsSlugs) == 0 && len(expectedSubjectsSlugs) > 0 {
		t.Errorf("Author has no subject slugs but documents have subject slugs: %v", expectedSubjectsSlugs)
	}
}
