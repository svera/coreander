package index

import (
	"log"
	"strings"

	"github.com/blevesearch/bleve/analysis/token/lowercase"
	"github.com/blevesearch/bleve/analysis/tokenizer/unicode"
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/custom"
	"github.com/blevesearch/bleve/v2/analysis/char/asciifolding"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/svera/coreander/internal/metadata"
)

const wordsPerMinute = 300.0

type BleveIndexer struct {
	idx         bleve.Index
	libraryPath string
	reader      map[string]metadata.Reader
}

// NewBleve creates a new BleveIndexer instance using the passed parameters
func NewBleve(index bleve.Index, libraryPath string, read map[string]metadata.Reader) *BleveIndexer {
	return &BleveIndexer{
		index,
		strings.TrimSuffix(libraryPath, "/"),
		read,
	}
}

func AddMappings(indexMapping *mapping.IndexMappingImpl) {
	err := indexMapping.AddCustomAnalyzer("book",
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
	indexMapping.DefaultAnalyzer = "book"
	languageFieldMapping := bleve.NewTextFieldMapping()
	languageFieldMapping.Index = false
	indexMapping.DefaultMapping.AddFieldMappingsAt("language", languageFieldMapping)
	yearFieldMapping := bleve.NewTextFieldMapping()
	yearFieldMapping.Index = false
	indexMapping.DefaultMapping.AddFieldMappingsAt("year", yearFieldMapping)
}

// Close closes the index
func (b *BleveIndexer) Close() error {
	return b.idx.Close()
}
