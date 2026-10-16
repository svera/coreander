package index_test

import (
	"fmt"
	"image"

	"github.com/pirmd/epub"
	"github.com/svera/coreander/v5/internal/metadata"
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
	md, err := metadata.BuildEpubMetadataFields(info, path)
	if err != nil {
		return metadata.Metadata{}, err
	}
	if w, ok := r.words[path]; ok {
		md.Words = w
	}
	return md, nil
}

func (r epubTestReader) Cover(string, int) (image.Image, error) {
	return nil, nil
}

// MetadataAndText, Text and RankText make epubTestReader satisfy
// metadata.TextExtractor, metadata.TextSource and metadata.TextRanker,
// mirroring the real EpubReader so tests exercise the same "needs TextRank
// enrichment" path as production .epub files.
func (r epubTestReader) MetadataAndText(path string) (metadata.Metadata, string, error) {
	md, err := r.Metadata(path)
	return md, "", err
}

func (r epubTestReader) Text(path string) (string, error) {
	return "", nil
}

func (r epubTestReader) RankText(minOccurrenceRatio float64, textContent, filename string) (*metadata.TextRankResult, error) {
	return nil, nil
}

func languageFilterLibrary() map[string]*epub.Information {
	return map[string]*epub.Information{
		"lib/english_book.epub": {
			Title:       []epub.MetadataValue{epub.NewMetadataValue("English Book")},
			Creator:     []epub.Author{{FullName: "English Author", Role: "aut"}},
			Description: []epub.MetadataValue{epub.NewMetadataValue("A book in English")},
			Language:    []epub.MetadataValue{epub.NewMetadataValue("en")},
			Subject:     []epub.MetadataValue{epub.NewMetadataValue("Fiction")},
		},
		"lib/spanish_book.epub": {
			Title:       []epub.MetadataValue{epub.NewMetadataValue("Spanish Book")},
			Creator:     []epub.Author{{FullName: "Spanish Author", Role: "aut"}},
			Description: []epub.MetadataValue{epub.NewMetadataValue("A book in Spanish")},
			Language:    []epub.MetadataValue{epub.NewMetadataValue("es")},
			Subject:     []epub.MetadataValue{epub.NewMetadataValue("Fiction")},
		},
		"lib/french_book.epub": {
			Title:       []epub.MetadataValue{epub.NewMetadataValue("French Book")},
			Creator:     []epub.Author{{FullName: "French Author", Role: "aut"}},
			Description: []epub.MetadataValue{epub.NewMetadataValue("A book in French")},
			Language:    []epub.MetadataValue{epub.NewMetadataValue("fr")},
			Subject:     []epub.MetadataValue{epub.NewMetadataValue("Fiction")},
		},
	}
}

func publicationDateSortLibrary() map[string]*epub.Information {
	return map[string]*epub.Information{
		"lib/oldest.epub": {
			Title: []epub.MetadataValue{epub.NewMetadataValue("Oldest Book")}, Creator: []epub.Author{{FullName: "Ancient Author", Role: "aut"}},
			Description: []epub.MetadataValue{epub.NewMetadataValue("The oldest book")}, Language: []epub.MetadataValue{epub.NewMetadataValue("en")}, Subject: []epub.MetadataValue{epub.NewMetadataValue("History")},
			Date: []epub.Date{{Stamp: "1800-01-01", Event: "publication"}},
		},
		"lib/older.epub": {
			Title: []epub.MetadataValue{epub.NewMetadataValue("Older Book")}, Creator: []epub.Author{{FullName: "Old Author", Role: "aut"}},
			Description: []epub.MetadataValue{epub.NewMetadataValue("An older book")}, Language: []epub.MetadataValue{epub.NewMetadataValue("en")}, Subject: []epub.MetadataValue{epub.NewMetadataValue("History")},
			Date: []epub.Date{{Stamp: "1900-06-15", Event: "publication"}},
		},
		"lib/newer.epub": {
			Title: []epub.MetadataValue{epub.NewMetadataValue("Newer Book")}, Creator: []epub.Author{{FullName: "New Author", Role: "aut"}},
			Description: []epub.MetadataValue{epub.NewMetadataValue("A newer book")}, Language: []epub.MetadataValue{epub.NewMetadataValue("en")}, Subject: []epub.MetadataValue{epub.NewMetadataValue("History")},
			Date: []epub.Date{{Stamp: "2000-12-31", Event: "publication"}},
		},
		"lib/newest.epub": {
			Title: []epub.MetadataValue{epub.NewMetadataValue("Newest Book")}, Creator: []epub.Author{{FullName: "Modern Author", Role: "aut"}},
			Description: []epub.MetadataValue{epub.NewMetadataValue("The newest book")}, Language: []epub.MetadataValue{epub.NewMetadataValue("en")}, Subject: []epub.MetadataValue{epub.NewMetadataValue("History")},
			Date: []epub.Date{{Stamp: "2023-03-20", Event: "publication"}},
		},
		"lib/no-date.epub": {
			Title: []epub.MetadataValue{epub.NewMetadataValue("No Date Book")}, Creator: []epub.Author{{FullName: "Unknown Author", Role: "aut"}},
			Description: []epub.MetadataValue{epub.NewMetadataValue("A book without publication date")}, Language: []epub.MetadataValue{epub.NewMetadataValue("en")}, Subject: []epub.MetadataValue{epub.NewMetadataValue("History")},
			Date: []epub.Date{},
		},
	}
}

func readingTimeSortReader() epubTestReader {
	info := map[string]*epub.Information{
		"lib/shortest.epub": {
			Title: []epub.MetadataValue{epub.NewMetadataValue("Shortest Book")}, Creator: []epub.Author{{FullName: "Short Author", Role: "aut"}},
			Description: []epub.MetadataValue{epub.NewMetadataValue("The shortest book")}, Language: []epub.MetadataValue{epub.NewMetadataValue("en")}, Subject: []epub.MetadataValue{epub.NewMetadataValue("Short Stories")},
			Date: []epub.Date{{Stamp: "2020-01-01", Event: "publication"}},
		},
		"lib/shorter.epub": {
			Title: []epub.MetadataValue{epub.NewMetadataValue("Shorter Book")}, Creator: []epub.Author{{FullName: "Medium Author", Role: "aut"}},
			Description: []epub.MetadataValue{epub.NewMetadataValue("A shorter book")}, Language: []epub.MetadataValue{epub.NewMetadataValue("en")}, Subject: []epub.MetadataValue{epub.NewMetadataValue("Short Stories")},
			Date: []epub.Date{{Stamp: "2020-06-15", Event: "publication"}},
		},
		"lib/longer.epub": {
			Title: []epub.MetadataValue{epub.NewMetadataValue("Longer Book")}, Creator: []epub.Author{{FullName: "Long Author", Role: "aut"}},
			Description: []epub.MetadataValue{epub.NewMetadataValue("A longer book")}, Language: []epub.MetadataValue{epub.NewMetadataValue("en")}, Subject: []epub.MetadataValue{epub.NewMetadataValue("Novels")},
			Date: []epub.Date{{Stamp: "2020-12-31", Event: "publication"}},
		},
		"lib/longest.epub": {
			Title: []epub.MetadataValue{epub.NewMetadataValue("Longest Book")}, Creator: []epub.Author{{FullName: "Epic Author", Role: "aut"}},
			Description: []epub.MetadataValue{epub.NewMetadataValue("The longest book")}, Language: []epub.MetadataValue{epub.NewMetadataValue("en")}, Subject: []epub.MetadataValue{epub.NewMetadataValue("Epic Novels")},
			Date: []epub.Date{{Stamp: "2023-03-20", Event: "publication"}},
		},
		"lib/no-words.epub": {
			Title: []epub.MetadataValue{epub.NewMetadataValue("No Words Book")}, Creator: []epub.Author{{FullName: "Unknown Author", Role: "aut"}},
			Description: []epub.MetadataValue{epub.NewMetadataValue("A book without word count")}, Language: []epub.MetadataValue{epub.NewMetadataValue("en")}, Subject: []epub.MetadataValue{epub.NewMetadataValue("Mystery")},
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
