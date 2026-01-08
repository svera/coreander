package index_test

import (
	"fmt"
	"html/template"
	"reflect"
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/pirmd/epub"
	"github.com/rickb777/date/v2"
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
			indexMem, err := bleve.NewMemOnly(index.CreateDocumentsMapping())
			if err != nil {
				t.Errorf("Error initialising index")
			}

			var mockMetadataReaders map[string]metadata.Reader

			// Use custom mockEpubReader for reading time tests that need specific word counts
			if tcase.name == "Test estimated reading time range search" {
				mockMetadataReaders = map[string]metadata.Reader{
					".epub": mockEpubReader{
						EpubReader: metadata.EpubReader{
							GetMetadataFromFile: func(file string) (*epub.Information, error) {
								return tcase.mockedMeta, nil
							},
							GetPackageFromFile: epub.GetPackageFromFile,
						},
					},
				}
			} else {
				mockMetadataReaders = map[string]metadata.Reader{
					".epub": metadata.EpubReader{
						GetMetadataFromFile: func(file string) (*epub.Information, error) {
							return tcase.mockedMeta, nil
						},
						GetPackageFromFile: epub.GetPackageFromFile,
					},
				}
			}

			appFS := afero.NewMemMapFs()
			// create test files and directories
			appFS.MkdirAll("lib", 0755)
			if err = afero.WriteFile(appFS, tcase.filename, []byte(""), 0644); err != nil {
				t.Errorf("Couldn't write file %s", tcase.filename)
			}

			authorsIndexMem, _ := bleve.NewMemOnly(index.CreateAuthorsMapping())
			idx := index.NewBleve(indexMem, authorsIndexMem, appFS, "lib", mockMetadataReaders)

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

func TestLanguageFilter(t *testing.T) {
	indexMem, err := bleve.NewMemOnly(index.CreateDocumentsMapping())
	if err != nil {
		t.Fatalf("Error initialising index: %v", err)
	}

	mockMetadataReaders := map[string]metadata.Reader{
		".epub": mockEpubReader{
			EpubReader: metadata.EpubReader{
				GetMetadataFromFile: func(file string) (*epub.Information, error) {
					switch file {
					case "lib/english_book.epub":
						return &epub.Information{
							Title:       []string{"English Book"},
							Creator:     []epub.Author{{FullName: "English Author", Role: "aut"}},
							Description: []string{"A book in English"},
							Language:    []string{"en"},
							Subject:     []string{"Fiction"},
						}, nil
					case "lib/spanish_book.epub":
						return &epub.Information{
							Title:       []string{"Spanish Book"},
							Creator:     []epub.Author{{FullName: "Spanish Author", Role: "aut"}},
							Description: []string{"A book in Spanish"},
							Language:    []string{"es"},
							Subject:     []string{"Fiction"},
						}, nil
					case "lib/french_book.epub":
						return &epub.Information{
							Title:       []string{"French Book"},
							Creator:     []epub.Author{{FullName: "French Author", Role: "aut"}},
							Description: []string{"A book in French"},
							Language:    []string{"fr"},
							Subject:     []string{"Fiction"},
						}, nil
					}
					return nil, fmt.Errorf("file not found")
				},
				GetPackageFromFile: epub.GetPackageFromFile,
			},
		},
	}

	appFS := afero.NewMemMapFs()
	appFS.MkdirAll("lib", 0755)
	afero.WriteFile(appFS, "lib/english_book.epub", []byte(""), 0644)
	afero.WriteFile(appFS, "lib/spanish_book.epub", []byte(""), 0644)
	afero.WriteFile(appFS, "lib/french_book.epub", []byte(""), 0644)

	authorsIndexMem, _ := bleve.NewMemOnly(index.CreateAuthorsMapping())
	idx := index.NewBleve(indexMem, authorsIndexMem, appFS, "lib", mockMetadataReaders)

	if err = idx.AddLibrary(1, true); err != nil {
		t.Fatalf("Error indexing: %s", err.Error())
	}

	// Test combining language filter with keyword search - Spanish
	res, err := idx.Search(index.SearchFields{Keywords: "book", Language: "es"}, 1, 10)
	if err != nil {
		t.Fatalf("Error searching: %s", err.Error())
	}
	if res.TotalHits() != 1 {
		t.Errorf("Expected 1 Spanish document, got %d", res.TotalHits())
	}
	if len(res.Hits()) > 0 && res.Hits()[0].Metadata.Language != "es" {
		t.Errorf("Expected Spanish language, got %s", res.Hits()[0].Metadata.Language)
	}

	// Test combining language filter with keyword search - English
	res, err = idx.Search(index.SearchFields{Keywords: "book", Language: "en"}, 1, 10)
	if err != nil {
		t.Fatalf("Error searching: %s", err.Error())
	}
	if res.TotalHits() != 1 {
		t.Errorf("Expected 1 English document, got %d", res.TotalHits())
	}
	if len(res.Hits()) > 0 && res.Hits()[0].Metadata.Language != "en" {
		t.Errorf("Expected English language, got %s", res.Hits()[0].Metadata.Language)
	}

	// Test searching by subject but no language filter - should return all matching documents
	res, err = idx.Search(index.SearchFields{Subjects: "Fiction"}, 1, 10)
	if err != nil {
		t.Fatalf("Error searching: %s", err.Error())
	}
	if res.TotalHits() != 3 {
		t.Errorf("Expected 3 documents (all languages with 'fiction' subject), got %d", res.TotalHits())
	}

	// Test combining language filter with subject search - French
	res, err = idx.Search(index.SearchFields{Subjects: "Fiction", Language: "fr"}, 1, 10)
	if err != nil {
		t.Fatalf("Error searching: %s", err.Error())
	}
	if res.TotalHits() != 1 {
		t.Errorf("Expected 1 French fiction document, got %d", res.TotalHits())
	}
	if len(res.Hits()) > 0 && res.Hits()[0].Metadata.Language != "fr" {
		t.Errorf("Expected French language, got %s", res.Hits()[0].Metadata.Language)
	}
}

func TestSearchResultsSortedByDate(t *testing.T) {
	indexMem, err := bleve.NewMemOnly(index.CreateDocumentsMapping())
	if err != nil {
		t.Fatalf("Error initialising index: %v", err)
	}

	mockMetadataReaders := map[string]metadata.Reader{
		".epub": metadata.EpubReader{
			GetMetadataFromFile: func(file string) (*epub.Information, error) {
				switch file {
				case "lib/oldest.epub":
					return &epub.Information{
						Title: []string{"Oldest Book"},
						Creator: []epub.Author{
							{FullName: "Ancient Author", Role: "aut"},
						},
						Description: []string{"The oldest book"},
						Language:    []string{"en"},
						Subject:     []string{"History"},
						Date: []epub.Date{
							{Stamp: "1800-01-01", Event: "publication"},
						},
					}, nil
				case "lib/older.epub":
					return &epub.Information{
						Title: []string{"Older Book"},
						Creator: []epub.Author{
							{FullName: "Old Author", Role: "aut"},
						},
						Description: []string{"An older book"},
						Language:    []string{"en"},
						Subject:     []string{"History"},
						Date: []epub.Date{
							{Stamp: "1900-06-15", Event: "publication"},
						},
					}, nil
				case "lib/newer.epub":
					return &epub.Information{
						Title: []string{"Newer Book"},
						Creator: []epub.Author{
							{FullName: "New Author", Role: "aut"},
						},
						Description: []string{"A newer book"},
						Language:    []string{"en"},
						Subject:     []string{"History"},
						Date: []epub.Date{
							{Stamp: "2000-12-31", Event: "publication"},
						},
					}, nil
				case "lib/newest.epub":
					return &epub.Information{
						Title: []string{"Newest Book"},
						Creator: []epub.Author{
							{FullName: "Modern Author", Role: "aut"},
						},
						Description: []string{"The newest book"},
						Language:    []string{"en"},
						Subject:     []string{"History"},
						Date: []epub.Date{
							{Stamp: "2023-03-20", Event: "publication"},
						},
					}, nil
				case "lib/no-date.epub":
					return &epub.Information{
						Title: []string{"No Date Book"},
						Creator: []epub.Author{
							{FullName: "Unknown Author", Role: "aut"},
						},
						Description: []string{"A book without publication date"},
						Language:    []string{"en"},
						Subject:     []string{"History"},
						Date:        []epub.Date{},
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
	testFiles := []string{"lib/oldest.epub", "lib/older.epub", "lib/newer.epub", "lib/newest.epub", "lib/no-date.epub"}
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

	t.Run("Test search results sorted by publication date older first", func(t *testing.T) {
		res, err := idx.Search(index.SearchFields{
			Subjects: "History",
			SortBy:   []string{"Publication.Date"},
		}, 1, 10)

		if err != nil {
			t.Fatalf("Error searching: %v", err)
		}

		if len(res.Hits()) != 5 {
			t.Fatalf("Expected 5 results, got %d", len(res.Hits()))
		}

		// Verify they are sorted from oldest to newest, with no-date books at the beginning (date value 0)
		expectedOrder := []string{"no-date.epub", "oldest.epub", "older.epub", "newer.epub", "newest.epub"}
		for i, doc := range res.Hits() {
			if doc.ID != expectedOrder[i] {
				t.Errorf("Expected document %s at position %d, got %s", expectedOrder[i], i, doc.ID)
			}
		}
	})

	t.Run("Test search results sorted by publication date newer first", func(t *testing.T) {
		res, err := idx.Search(index.SearchFields{
			Subjects: "History",
			SortBy:   []string{"-Publication.Date"},
		}, 1, 10)

		if err != nil {
			t.Fatalf("Error searching: %v", err)
		}

		if len(res.Hits()) != 5 {
			t.Fatalf("Expected 5 results, got %d", len(res.Hits()))
		}

		// Verify they are sorted from newest to oldest, with no-date books at the end
		expectedOrder := []string{"newest.epub", "newer.epub", "older.epub", "oldest.epub", "no-date.epub"}
		for i, doc := range res.Hits() {
			if doc.ID != expectedOrder[i] {
				t.Errorf("Expected document %s at position %d, got %s", expectedOrder[i], i, doc.ID)
			}
		}
	})
}

// Create a custom metadata reader that sets different word counts
type mockEpubReader struct {
	metadata.EpubReader
}

func (m mockEpubReader) Metadata(filename string) (metadata.Metadata, error) {
	// Get the base metadata from the mock
	meta, err := m.GetMetadataFromFile(filename)
	if err != nil {
		return metadata.Metadata{}, err
	}

	// Create metadata with custom word counts based on filename
	var wordCount float64
	switch filename {
	case "lib/shortest.epub":
		wordCount = 1000 // 1000 words
	case "lib/shorter.epub":
		wordCount = 5000 // 5000 words
	case "lib/longer.epub":
		wordCount = 15000 // 15000 words
	case "lib/longest.epub":
		wordCount = 50000 // 50000 words
	case "lib/no-words.epub":
		wordCount = 0 // 0 words
	case "lib/book19.epub":
		wordCount = 24000 // 24000 words = 2 hours at 200 wpm
	default:
		wordCount = 1000 // Default to 1000 words for other files
	}

	// Create the metadata manually
	title := meta.Title[0]
	var authors []string
	for _, creator := range meta.Creator {
		if creator.Role == "aut" || creator.Role == "" {
			authors = append(authors, creator.FullName)
		}
	}
	if len(authors) == 0 {
		authors = []string{""}
	}

	description := ""
	if len(meta.Description) > 0 {
		description = "<p>" + meta.Description[0] + "</p>"
	}

	lang := ""
	if len(meta.Language) > 0 {
		lang = meta.Language[0]
	}

	publication := precisiondate.PrecisionDate{Precision: precisiondate.PrecisionDay}
	for _, currentDate := range meta.Date {
		if currentDate.Event == "publication" || currentDate.Event == "" {
			if publication.Date, err = date.ParseISO(currentDate.Stamp); err != nil {
				publication.Precision = precisiondate.PrecisionYear
				publication.Date, _ = date.Parse("2006", currentDate.Stamp)
			}
			break
		}
	}

	return metadata.Metadata{
		Title:       title,
		Authors:     authors,
		Description: template.HTML(description),
		Language:    lang,
		Publication: publication,
		Series:      meta.Series,
		SeriesIndex: 0,
		Format:      "EPUB",
		Subjects:    meta.Subject,
		Words:       wordCount,
	}, nil
}

func TestSearchResultsSortedByReadingTime(t *testing.T) {
	indexMem, err := bleve.NewMemOnly(index.CreateDocumentsMapping())
	if err != nil {
		t.Fatalf("Error initialising index: %v", err)
	}

	mockMetadataReaders := map[string]metadata.Reader{
		".epub": mockEpubReader{
			EpubReader: metadata.EpubReader{
				GetMetadataFromFile: func(file string) (*epub.Information, error) {
					switch file {
					case "lib/shortest.epub":
						return &epub.Information{
							Title: []string{"Shortest Book"},
							Creator: []epub.Author{
								{FullName: "Short Author", Role: "aut"},
							},
							Description: []string{"The shortest book"},
							Language:    []string{"en"},
							Subject:     []string{"Short Stories"},
							Date: []epub.Date{
								{Stamp: "2020-01-01", Event: "publication"},
							},
						}, nil
					case "lib/shorter.epub":
						return &epub.Information{
							Title: []string{"Shorter Book"},
							Creator: []epub.Author{
								{FullName: "Medium Author", Role: "aut"},
							},
							Description: []string{"A shorter book"},
							Language:    []string{"en"},
							Subject:     []string{"Short Stories"},
							Date: []epub.Date{
								{Stamp: "2020-06-15", Event: "publication"},
							},
						}, nil
					case "lib/longer.epub":
						return &epub.Information{
							Title: []string{"Longer Book"},
							Creator: []epub.Author{
								{FullName: "Long Author", Role: "aut"},
							},
							Description: []string{"A longer book"},
							Language:    []string{"en"},
							Subject:     []string{"Novels"},
							Date: []epub.Date{
								{Stamp: "2020-12-31", Event: "publication"},
							},
						}, nil
					case "lib/longest.epub":
						return &epub.Information{
							Title: []string{"Longest Book"},
							Creator: []epub.Author{
								{FullName: "Epic Author", Role: "aut"},
							},
							Description: []string{"The longest book"},
							Language:    []string{"en"},
							Subject:     []string{"Epic Novels"},
							Date: []epub.Date{
								{Stamp: "2023-03-20", Event: "publication"},
							},
						}, nil
					case "lib/no-words.epub":
						return &epub.Information{
							Title: []string{"No Words Book"},
							Creator: []epub.Author{
								{FullName: "Unknown Author", Role: "aut"},
							},
							Description: []string{"A book without word count"},
							Language:    []string{"en"},
							Subject:     []string{"Mystery"},
							Date:        []epub.Date{},
						}, nil
					default:
						return nil, fmt.Errorf("unknown file: %s", file)
					}
				},
				GetPackageFromFile: epub.GetPackageFromFile,
			},
		},
	}

	appFS := afero.NewMemMapFs()
	appFS.MkdirAll("lib", 0755)

	// Create test files
	testFiles := []string{"lib/shortest.epub", "lib/shorter.epub", "lib/longer.epub", "lib/longest.epub", "lib/no-words.epub"}
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

	t.Run("Test search results sorted by reading time shorter first", func(t *testing.T) {
		res, err := idx.Search(index.SearchFields{
			Keywords: "book",
			SortBy:   []string{"Words"},
		}, 1, 10)

		if err != nil {
			t.Fatalf("Error searching: %v", err)
		}

		if len(res.Hits()) != 5 {
			t.Fatalf("Expected 5 results, got %d", len(res.Hits()))
		}

		// Verify they are sorted from shortest to longest, with no-words books at the beginning (words value 0)
		expectedOrder := []string{"no-words.epub", "shortest.epub", "shorter.epub", "longer.epub", "longest.epub"}
		for i, doc := range res.Hits() {
			if doc.ID != expectedOrder[i] {
				t.Errorf("Expected document %s at position %d, got %s", expectedOrder[i], i, doc.ID)
			}
		}
	})

	t.Run("Test search results sorted by reading time longer first", func(t *testing.T) {
		res, err := idx.Search(index.SearchFields{
			Keywords: "book",
			SortBy:   []string{"-Words"},
		}, 1, 10)

		if err != nil {
			t.Fatalf("Error searching: %v", err)
		}

		if len(res.Hits()) != 5 {
			t.Fatalf("Expected 5 results, got %d", len(res.Hits()))
		}

		// Verify they are sorted from longest to shortest, with no-words books at the end
		expectedOrder := []string{"longest.epub", "longer.epub", "shorter.epub", "shortest.epub", "no-words.epub"}
		for i, doc := range res.Hits() {
			if doc.ID != expectedOrder[i] {
				t.Errorf("Expected document %s at position %d, got %s", expectedOrder[i], i, doc.ID)
			}
		}
	})
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
							Language:    "es",
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
							Language:    "fr",
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
							Language:    "en",
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
							Language:    "en",
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
							Language:    "en",
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
							Language:    "es",
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
							Language:    "it",
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
							Language:    "es",
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
							Language:    "es",
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
							Language:    "es",
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
							Language:    "es",
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
			"Test date range search",
			"lib/book11.epub",
			&epub.Information{
				Title: []string{"Modern History Book"},
				Creator: []epub.Author{
					{
						FullName: "John Smith",
						Role:     "aut",
					},
				},
				Description: []string{"A book about modern history"},
				Language:    []string{"en"},
				Subject:     []string{"History", "Modern"},
				Date: []epub.Date{
					{
						Stamp: "2020-06-15",
						Event: "publication",
					},
				},
			},
			index.SearchFields{
				Keywords:    "",
				PubDateFrom: date.New(2020, 1, 1),
				PubDateTo:   date.New(2020, 12, 31),
			},
			result.NewPaginated(
				model.ResultsPerPage,
				1,
				1,
				[]index.Document{
					{
						ID:   "book11.epub",
						Slug: "john-smith-modern-history-book",
						Metadata: metadata.Metadata{
							Title:       "Modern History Book",
							Authors:     []string{"John Smith"},
							Description: "<p>A book about modern history</p>",
							Language:    "en",
							Subjects:    []string{"History", "Modern"},
							Format:      "EPUB",
							Publication: precisiondate.NewPrecisionDate("2020-06-15T00:00:00Z", precisiondate.PrecisionDay),
						},
						AuthorsSlugs:  []string{"john-smith"},
						SeriesSlug:    "",
						SubjectsSlugs: []string{"history", "modern"},
					},
				},
			),
		},
		{
			"Test date range search with year precision",
			"lib/book12.epub",
			&epub.Information{
				Title: []string{"Ancient History Book"},
				Creator: []epub.Author{
					{
						FullName: "Jane Doe",
						Role:     "aut",
					},
				},
				Description: []string{"A book about ancient history"},
				Language:    []string{"en"},
				Subject:     []string{"History", "Ancient"},
				Date: []epub.Date{
					{
						Stamp: "1975",
						Event: "publication",
					},
				},
			},
			index.SearchFields{
				Keywords:    "",
				PubDateFrom: date.New(1970, 1, 1),
				PubDateTo:   date.New(1980, 1, 1),
			},
			result.NewPaginated(
				model.ResultsPerPage,
				1,
				1,
				[]index.Document{
					{
						ID:   "book12.epub",
						Slug: "jane-doe-ancient-history-book",
						Metadata: metadata.Metadata{
							Title:       "Ancient History Book",
							Authors:     []string{"Jane Doe"},
							Description: "<p>A book about ancient history</p>",
							Language:    "en",
							Subjects:    []string{"History", "Ancient"},
							Format:      "EPUB",
							Publication: precisiondate.NewPrecisionDate("1975-01-01T00:00:00Z", precisiondate.PrecisionYear),
						},
						AuthorsSlugs:  []string{"jane-doe"},
						SeriesSlug:    "",
						SubjectsSlugs: []string{"history", "ancient"},
					},
				},
			),
		},
		{
			"Test search results sorted by publication date older first",
			"lib/book13.epub",
			&epub.Information{
				Title: []string{"Old Book"},
				Creator: []epub.Author{
					{
						FullName: "Ancient Author",
						Role:     "aut",
					},
				},
				Description: []string{"An old book"},
				Language:    []string{"en"},
				Subject:     []string{"History"},
				Date: []epub.Date{
					{
						Stamp: "1800-01-01",
						Event: "publication",
					},
				},
			},
			index.SearchFields{
				Subjects: "History",
				SortBy:   []string{"Publication.Date"},
			},
			result.NewPaginated(
				model.ResultsPerPage,
				1,
				1,
				[]index.Document{
					{
						ID:   "book13.epub",
						Slug: "ancient-author-old-book",
						Metadata: metadata.Metadata{
							Title:       "Old Book",
							Authors:     []string{"Ancient Author"},
							Description: "<p>An old book</p>",
							Language:    "en",
							Subjects:    []string{"History"},
							Format:      "EPUB",
							Publication: precisiondate.NewPrecisionDate("1800-01-01T00:00:00Z", precisiondate.PrecisionDay),
						},
						AuthorsSlugs:  []string{"ancient-author"},
						SeriesSlug:    "",
						SubjectsSlugs: []string{"history"},
					},
				},
			),
		},
		{
			"Test search results sorted by publication date newer first",
			"lib/book14.epub",
			&epub.Information{
				Title: []string{"New Book"},
				Creator: []epub.Author{
					{
						FullName: "Modern Author",
						Role:     "aut",
					},
				},
				Description: []string{"A new book"},
				Language:    []string{"en"},
				Subject:     []string{"Technology"},
				Date: []epub.Date{
					{
						Stamp: "2023-12-31",
						Event: "publication",
					},
				},
			},
			index.SearchFields{
				Subjects: "Technology",
				SortBy:   []string{"-Publication.Date"},
			},
			result.NewPaginated(
				model.ResultsPerPage,
				1,
				1,
				[]index.Document{
					{
						ID:   "book14.epub",
						Slug: "modern-author-new-book",
						Metadata: metadata.Metadata{
							Title:       "New Book",
							Authors:     []string{"Modern Author"},
							Description: "<p>A new book</p>",
							Language:    "en",
							Subjects:    []string{"Technology"},
							Format:      "EPUB",
							Publication: precisiondate.NewPrecisionDate("2023-12-31T00:00:00Z", precisiondate.PrecisionDay),
						},
						AuthorsSlugs:  []string{"modern-author"},
						SeriesSlug:    "",
						SubjectsSlugs: []string{"technology"},
					},
				},
			),
		},
		{
			"Test multiple books sorted by publication date older first",
			"lib/book15.epub",
			&epub.Information{
				Title: []string{"Middle Book"},
				Creator: []epub.Author{
					{
						FullName: "Middle Author",
						Role:     "aut",
					},
				},
				Description: []string{"A middle-aged book"},
				Language:    []string{"en"},
				Subject:     []string{"Literature"},
				Date: []epub.Date{
					{
						Stamp: "1950-06-15",
						Event: "publication",
					},
				},
			},
			index.SearchFields{
				Subjects: "Literature",
				SortBy:   []string{"Publication.Date"},
			},
			result.NewPaginated(
				model.ResultsPerPage,
				1,
				1,
				[]index.Document{
					{
						ID:   "book15.epub",
						Slug: "middle-author-middle-book",
						Metadata: metadata.Metadata{
							Title:       "Middle Book",
							Authors:     []string{"Middle Author"},
							Description: "<p>A middle-aged book</p>",
							Language:    "en",
							Subjects:    []string{"Literature"},
							Format:      "EPUB",
							Publication: precisiondate.NewPrecisionDate("1950-06-15T00:00:00Z", precisiondate.PrecisionDay),
						},
						AuthorsSlugs:  []string{"middle-author"},
						SeriesSlug:    "",
						SubjectsSlugs: []string{"literature"},
					},
				},
			),
		},
		{
			"Test books with different date precisions sorted by publication date",
			"lib/book16.epub",
			&epub.Information{
				Title: []string{"Decade Book"},
				Creator: []epub.Author{
					{
						FullName: "Decade Author",
						Role:     "aut",
					},
				},
				Description: []string{"A book from a specific decade"},
				Language:    []string{"en"},
				Subject:     []string{"History"},
				Date: []epub.Date{
					{
						Stamp: "1980",
						Event: "publication",
					},
				},
			},
			index.SearchFields{
				Subjects: "History",
				SortBy:   []string{"Publication.Date"},
			},
			result.NewPaginated(
				model.ResultsPerPage,
				1,
				1,
				[]index.Document{
					{
						ID:   "book16.epub",
						Slug: "decade-author-decade-book",
						Metadata: metadata.Metadata{
							Title:       "Decade Book",
							Authors:     []string{"Decade Author"},
							Description: "<p>A book from a specific decade</p>",
							Language:    "en",
							Subjects:    []string{"History"},
							Format:      "EPUB",
							Publication: precisiondate.NewPrecisionDate("1980-01-01T00:00:00Z", precisiondate.PrecisionYear),
						},
						AuthorsSlugs:  []string{"decade-author"},
						SeriesSlug:    "",
						SubjectsSlugs: []string{"history"},
					},
				},
			),
		},
		{
			"Test search results sorted by reading time shorter first",
			"lib/book17.epub",
			&epub.Information{
				Title: []string{"Short Book"},
				Creator: []epub.Author{
					{
						FullName: "Short Author",
						Role:     "aut",
					},
				},
				Description: []string{"A short book"},
				Language:    []string{"en"},
				Subject:     []string{"Short Stories"},
				Date: []epub.Date{
					{
						Stamp: "2020-01-01",
						Event: "publication",
					},
				},
			},
			index.SearchFields{
				Keywords: "short",
				SortBy:   []string{"Words"},
			},
			result.NewPaginated(
				model.ResultsPerPage,
				1,
				1,
				[]index.Document{
					{
						ID:   "book17.epub",
						Slug: "short-author-short-book",
						Metadata: metadata.Metadata{
							Title:       "Short Book",
							Authors:     []string{"Short Author"},
							Description: "<p>A short book</p>",
							Language:    "en",
							Subjects:    []string{"Short Stories"},
							Format:      "EPUB",
							Publication: precisiondate.NewPrecisionDate("2020-01-01T00:00:00Z", precisiondate.PrecisionDay),
						},
						AuthorsSlugs:  []string{"short-author"},
						SeriesSlug:    "",
						SubjectsSlugs: []string{"short-stories"},
					},
				},
			),
		},
		{
			"Test search results sorted by reading time longer first",
			"lib/book18.epub",
			&epub.Information{
				Title: []string{"Long Book"},
				Creator: []epub.Author{
					{
						FullName: "Long Author",
						Role:     "aut",
					},
				},
				Description: []string{"A long book"},
				Language:    []string{"en"},
				Subject:     []string{"Novels"},
				Date: []epub.Date{
					{
						Stamp: "2020-12-31",
						Event: "publication",
					},
				},
			},
			index.SearchFields{
				Keywords: "long",
				SortBy:   []string{"-Words"},
			},
			result.NewPaginated(
				model.ResultsPerPage,
				1,
				1,
				[]index.Document{
					{
						ID:   "book18.epub",
						Slug: "long-author-long-book",
						Metadata: metadata.Metadata{
							Title:       "Long Book",
							Authors:     []string{"Long Author"},
							Description: "<p>A long book</p>",
							Language:    "en",
							Subjects:    []string{"Novels"},
							Format:      "EPUB",
							Publication: precisiondate.NewPrecisionDate("2020-12-31T00:00:00Z", precisiondate.PrecisionDay),
						},
						AuthorsSlugs:  []string{"long-author"},
						SeriesSlug:    "",
						SubjectsSlugs: []string{"novels"},
					},
				},
			),
		},
		{
			"Test estimated reading time range search",
			"lib/book19.epub",
			&epub.Information{
				Title: []string{"Medium Length Book"},
				Creator: []epub.Author{
					{
						FullName: "Medium Author",
						Role:     "aut",
					},
				},
				Description: []string{"A medium length book"},
				Language:    []string{"en"},
				Subject:     []string{"Fiction"},
				Date: []epub.Date{
					{
						Stamp: "2021-06-15",
						Event: "publication",
					},
				},
			},
			index.SearchFields{
				Keywords:        "",
				EstReadTimeFrom: 1.0, // 1 hour minimum
				EstReadTimeTo:   3.0, // 3 hours maximum
				WordsPerMinute:  200.0,
			},
			result.NewPaginated(
				model.ResultsPerPage,
				1,
				1,
				[]index.Document{
					{
						ID:   "book19.epub",
						Slug: "medium-author-medium-length-book",
						Metadata: metadata.Metadata{
							Title:       "Medium Length Book",
							Authors:     []string{"Medium Author"},
							Description: "<p>A medium length book</p>",
							Language:    "en",
							Subjects:    []string{"Fiction"},
							Format:      "EPUB",
							Publication: precisiondate.NewPrecisionDate("2021-06-15T00:00:00Z", precisiondate.PrecisionDay),
							Words:       24000, // 24000 words = 2 hours at 200 wpm
						},
						AuthorsSlugs:  []string{"medium-author"},
						SeriesSlug:    "",
						SubjectsSlugs: []string{"fiction"},
					},
				},
			),
		},
		{
			"Test language filter - search for Spanish documents only",
			"lib/book_spanish.epub",
			&epub.Information{
				Title: []string{"Spanish Book"},
				Creator: []epub.Author{
					{
						FullName: "Spanish Author",
						Role:     "aut",
					},
				},
				Description: []string{"A book in Spanish"},
				Language:    []string{"es"},
				Subject:     []string{"Literature"},
				Date: []epub.Date{
					{
						Stamp: "2023-01-01",
						Event: "publication",
					},
				},
			},
			index.SearchFields{
				Keywords: "",
				Language: "es",
			},
			result.NewPaginated(
				model.ResultsPerPage,
				1,
				1,
				[]index.Document{
					{
						ID:   "book_spanish.epub",
						Slug: "spanish-author-spanish-book",
						Metadata: metadata.Metadata{
							Title:       "Spanish Book",
							Authors:     []string{"Spanish Author"},
							Description: "<p>A book in Spanish</p>",
							Language:    "es",
							Subjects:    []string{"Literature"},
							Format:      "EPUB",
							Publication: precisiondate.NewPrecisionDate("2023-01-01T00:00:00Z", precisiondate.PrecisionDay),
						},
						AuthorsSlugs:  []string{"spanish-author"},
						SeriesSlug:    "",
						SubjectsSlugs: []string{"literature"},
					},
				},
			),
		},
	}
}
