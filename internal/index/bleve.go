package index

import (
	"log"
	"path/filepath"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/custom"
	"github.com/blevesearch/bleve/v2/analysis/char/asciifolding"
	"github.com/blevesearch/bleve/v2/analysis/lang/en"
	"github.com/blevesearch/bleve/v2/analysis/lang/es"
	"github.com/blevesearch/bleve/v2/analysis/token/lowercase"
	"github.com/blevesearch/bleve/v2/analysis/tokenizer/unicode"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/svera/coreander/v3/internal/metadata"
)

var languages = []string{es.AnalyzerName, en.AnalyzerName}

type BleveIndexer struct {
	idx         bleve.Index
	libraryPath string
	reader      map[string]metadata.Reader
}

// NewBleve creates a new BleveIndexer instance using the passed parameters
func NewBleve(index bleve.Index, libraryPath string, read map[string]metadata.Reader) *BleveIndexer {
	return &BleveIndexer{
		index,
		strings.TrimSuffix(libraryPath, string(filepath.Separator)),
		read,
	}
}

func Mapping() mapping.IndexMapping {
	indexMapping := bleve.NewIndexMapping()

	err := indexMapping.AddCustomAnalyzer("document",
		map[string]interface{}{
			"type": custom.Name,
			"char_filters": []string{
				asciifolding.Name,
			},
			"tokenizer": unicode.Name,
			"token_filters": []string{
				lowercase.Name,
			},
		})
	if err != nil {
		log.Fatal(err)
	}

	keywordFieldMapping := bleve.NewKeywordFieldMapping()
	//keywordFieldMapping.Analyzer = "document"
	keywordFieldMappingNotIndexable := bleve.NewKeywordFieldMapping()
	//keywordFieldMappingNotIndexable.Analyzer = "document"

	for _, lang := range languages {
		textFieldMapping := bleve.NewTextFieldMapping()
		textFieldMapping.Analyzer = lang

		simpleTextFieldMapping := bleve.NewTextFieldMapping()
		simpleTextFieldMapping.Analyzer = "document"

		err := addNoStopWordsAnalyzer(lang, indexMapping)
		if err != nil {
			log.Fatal(err)
		}
		noStopWordsTextFieldMapping := bleve.NewTextFieldMapping()
		noStopWordsTextFieldMapping.Analyzer = lang + "_no_stop_words"

		indexMapping.AddDocumentMapping(lang, bleve.NewDocumentMapping())
		indexMapping.TypeMapping[lang].DefaultAnalyzer = lang
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Title", noStopWordsTextFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Authors", simpleTextFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Description", textFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Subjects", textFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Series", noStopWordsTextFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Slug", keywordFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("SeriesEq", keywordFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("AuthorsEq", keywordFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("SubjectsEq", keywordFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Language", keywordFieldMappingNotIndexable)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Year", keywordFieldMappingNotIndexable)
	}

	indexMapping.DefaultMapping.DefaultAnalyzer = "document"
	//indexMapping.DefaultMapping.AddFieldMappingsAt("Authors", bleve.NewTextFieldMapping())
	//indexMapping.DefaultMapping.AddFieldMappingsAt("Slug", keywordFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("SeriesEq", keywordFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("AuthorsEq", keywordFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("SubjectsEq", keywordFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Language", keywordFieldMappingNotIndexable)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Year", keywordFieldMappingNotIndexable)

	return indexMapping
}

// Close closes the index
func (b *BleveIndexer) Close() error {
	return b.idx.Close()
}
