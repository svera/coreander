package index_test

import (
	"fmt"

	"github.com/pirmd/epub"
	"github.com/svera/coreander/v4/internal/metadata"
)

// epubTestReader implements metadata.Reader for index tests without injecting EpubReader hooks.
type epubTestReader struct {
	info  map[string]*epub.Information
	words map[string]float64
}

func (r epubTestReader) Metadata(path string) (metadata.Metadata, error) {
	info, ok := r.info[path]
	if !ok {
		return metadata.Metadata{}, fmt.Errorf("no test metadata for %s", path)
	}
	md, err := metadata.FromEpubInformation(path, info)
	if err != nil {
		return metadata.Metadata{}, err
	}
	if w, ok := r.words[path]; ok {
		md.Words = w
	}
	return md, nil
}

func (r epubTestReader) Cover(string, int) ([]byte, error) {
	return nil, nil
}

func languageFilterLibrary() map[string]*epub.Information {
	return map[string]*epub.Information{
		"lib/english_book.epub": {
			Title:       []string{"English Book"},
			Creator:     []epub.Author{{FullName: "English Author", Role: "aut"}},
			Description: []string{"A book in English"},
			Language:    []string{"en"},
			Subject:     []string{"Fiction"},
		},
		"lib/spanish_book.epub": {
			Title:       []string{"Spanish Book"},
			Creator:     []epub.Author{{FullName: "Spanish Author", Role: "aut"}},
			Description: []string{"A book in Spanish"},
			Language:    []string{"es"},
			Subject:     []string{"Fiction"},
		},
		"lib/french_book.epub": {
			Title:       []string{"French Book"},
			Creator:     []epub.Author{{FullName: "French Author", Role: "aut"}},
			Description: []string{"A book in French"},
			Language:    []string{"fr"},
			Subject:     []string{"Fiction"},
		},
	}
}

func publicationDateSortLibrary() map[string]*epub.Information {
	return map[string]*epub.Information{
		"lib/oldest.epub": {
			Title: []string{"Oldest Book"}, Creator: []epub.Author{{FullName: "Ancient Author", Role: "aut"}},
			Description: []string{"The oldest book"}, Language: []string{"en"}, Subject: []string{"History"},
			Date: []epub.Date{{Stamp: "1800-01-01", Event: "publication"}},
		},
		"lib/older.epub": {
			Title: []string{"Older Book"}, Creator: []epub.Author{{FullName: "Old Author", Role: "aut"}},
			Description: []string{"An older book"}, Language: []string{"en"}, Subject: []string{"History"},
			Date: []epub.Date{{Stamp: "1900-06-15", Event: "publication"}},
		},
		"lib/newer.epub": {
			Title: []string{"Newer Book"}, Creator: []epub.Author{{FullName: "New Author", Role: "aut"}},
			Description: []string{"A newer book"}, Language: []string{"en"}, Subject: []string{"History"},
			Date: []epub.Date{{Stamp: "2000-12-31", Event: "publication"}},
		},
		"lib/newest.epub": {
			Title: []string{"Newest Book"}, Creator: []epub.Author{{FullName: "Modern Author", Role: "aut"}},
			Description: []string{"The newest book"}, Language: []string{"en"}, Subject: []string{"History"},
			Date: []epub.Date{{Stamp: "2023-03-20", Event: "publication"}},
		},
		"lib/no-date.epub": {
			Title: []string{"No Date Book"}, Creator: []epub.Author{{FullName: "Unknown Author", Role: "aut"}},
			Description: []string{"A book without publication date"}, Language: []string{"en"}, Subject: []string{"History"},
			Date: []epub.Date{},
		},
	}
}

func readingTimeSortReader() epubTestReader {
	info := map[string]*epub.Information{
		"lib/shortest.epub": {
			Title: []string{"Shortest Book"}, Creator: []epub.Author{{FullName: "Short Author", Role: "aut"}},
			Description: []string{"The shortest book"}, Language: []string{"en"}, Subject: []string{"Short Stories"},
			Date: []epub.Date{{Stamp: "2020-01-01", Event: "publication"}},
		},
		"lib/shorter.epub": {
			Title: []string{"Shorter Book"}, Creator: []epub.Author{{FullName: "Medium Author", Role: "aut"}},
			Description: []string{"A shorter book"}, Language: []string{"en"}, Subject: []string{"Short Stories"},
			Date: []epub.Date{{Stamp: "2020-06-15", Event: "publication"}},
		},
		"lib/longer.epub": {
			Title: []string{"Longer Book"}, Creator: []epub.Author{{FullName: "Long Author", Role: "aut"}},
			Description: []string{"A longer book"}, Language: []string{"en"}, Subject: []string{"Novels"},
			Date: []epub.Date{{Stamp: "2020-12-31", Event: "publication"}},
		},
		"lib/longest.epub": {
			Title: []string{"Longest Book"}, Creator: []epub.Author{{FullName: "Epic Author", Role: "aut"}},
			Description: []string{"The longest book"}, Language: []string{"en"}, Subject: []string{"Epic Novels"},
			Date: []epub.Date{{Stamp: "2023-03-20", Event: "publication"}},
		},
		"lib/no-words.epub": {
			Title: []string{"No Words Book"}, Creator: []epub.Author{{FullName: "Unknown Author", Role: "aut"}},
			Description: []string{"A book without word count"}, Language: []string{"en"}, Subject: []string{"Mystery"},
			Date: []epub.Date{},
		},
	}
	return epubTestReader{
		info: info,
		words: map[string]float64{
			"lib/shortest.epub": 1000,
			"lib/shorter.epub":  5000,
			"lib/longer.epub":   15000,
			"lib/longest.epub":  50000,
			"lib/no-words.epub": 0,
		},
	}
}
