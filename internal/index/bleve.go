package index

import (
	"log"
	"path/filepath"
	"strings"

	"github.com/blevesearch/bleve/analysis/token/lowercase"
	"github.com/blevesearch/bleve/analysis/tokenizer/unicode"
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/custom"
	"github.com/blevesearch/bleve/v2/analysis/char/asciifolding"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/svera/coreander/v4/internal/metadata"
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
	languageFieldMapping := bleve.NewTextFieldMapping()
	languageFieldMapping.Index = false
	indexMapping.DefaultMapping.AddFieldMappingsAt("Language", languageFieldMapping)
	yearFieldMapping := bleve.NewTextFieldMapping()
	yearFieldMapping.Index = false
	indexMapping.DefaultMapping.AddFieldMappingsAt("Year", yearFieldMapping)
	slugFieldMapping := bleve.NewKeywordFieldMapping()
	indexMapping.DefaultMapping.AddFieldMappingsAt("Slug", slugFieldMapping)
	seriesEqFieldMapping := bleve.NewKeywordFieldMapping()
	indexMapping.DefaultMapping.AddFieldMappingsAt("SeriesEq", seriesEqFieldMapping)
	authorsEqFieldMapping := bleve.NewKeywordFieldMapping()
	indexMapping.DefaultMapping.AddFieldMappingsAt("AuthorsEq", authorsEqFieldMapping)
	subjectsEqFieldMapping := bleve.NewKeywordFieldMapping()
	indexMapping.DefaultMapping.AddFieldMappingsAt("SubjectsEq", subjectsEqFieldMapping)

	return indexMapping
}

// Close closes the index
func (b *BleveIndexer) Close() error {
	return b.idx.Close()
}
