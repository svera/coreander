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

func Mapping() *mapping.IndexMappingImpl {
	indexMapping := bleve.NewIndexMapping()

	languages := []string{es.AnalyzerName, en.AnalyzerName}

	keywordFieldMapping := bleve.NewKeywordFieldMapping()
	keywordFieldMappingNotIndexable := bleve.NewKeywordFieldMapping()

	for _, lang := range languages {
		documentMapping := bleve.NewDocumentMapping()
		documentMapping.DefaultAnalyzer = lang
		textFieldMapping := bleve.NewTextFieldMapping()
		textFieldMapping.Analyzer = lang

		documentMapping.AddFieldMappingsAt("Title", textFieldMapping)
		documentMapping.AddFieldMappingsAt("Description", textFieldMapping)
		documentMapping.AddFieldMappingsAt("Subjects", textFieldMapping)
		documentMapping.AddFieldMappingsAt("Slug", keywordFieldMapping)
		documentMapping.AddFieldMappingsAt("SeriesEq", keywordFieldMapping)
		documentMapping.AddFieldMappingsAt("AuthorsEq", keywordFieldMapping)
		documentMapping.AddFieldMappingsAt("SubjectsEq", keywordFieldMapping)
		documentMapping.AddFieldMappingsAt("Language", keywordFieldMappingNotIndexable)
		documentMapping.AddFieldMappingsAt("Year", keywordFieldMappingNotIndexable)

		indexMapping.AddDocumentMapping(lang, documentMapping)
	}

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

	indexMapping.DefaultAnalyzer = "document"
	//indexMapping.DefaultMapping = esMapping

	return indexMapping
}

// Close closes the index
func (b *BleveIndexer) Close() error {
	return b.idx.Close()
}
