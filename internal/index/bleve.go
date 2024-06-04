package index

import (
	"log"
	"path/filepath"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/custom"
	"github.com/blevesearch/bleve/v2/analysis/char/asciifolding"
	"github.com/blevesearch/bleve/v2/analysis/lang/de"
	"github.com/blevesearch/bleve/v2/analysis/lang/en"
	"github.com/blevesearch/bleve/v2/analysis/lang/es"
	"github.com/blevesearch/bleve/v2/analysis/lang/fr"
	"github.com/blevesearch/bleve/v2/analysis/lang/it"
	"github.com/blevesearch/bleve/v2/analysis/lang/pt"
	"github.com/blevesearch/bleve/v2/analysis/token/lowercase"
	"github.com/blevesearch/bleve/v2/analysis/token/porter"
	"github.com/blevesearch/bleve/v2/analysis/tokenizer/unicode"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/metadata"
)

// Version identifies the mapping used for indexing. Any changes in the mapping requires an increase
// of version, to signal that a new index needs to be created.
const Version = "v2"

// Metadata fields
var (
	internalLanguages = []byte("languages")
	internalVersion   = []byte("version")
)

var noStopWordsFilters = map[string][]string{
	es.AnalyzerName: {es.NormalizeName, lowercase.Name, es.LightStemmerName},
	en.AnalyzerName: {en.PossessiveName, lowercase.Name, porter.Name},
	de.AnalyzerName: {de.NormalizeName, lowercase.Name, de.LightStemmerName},
	fr.AnalyzerName: {fr.ElisionName, lowercase.Name, fr.LightStemmerName},
	it.AnalyzerName: {it.ElisionName, lowercase.Name, it.LightStemmerName},
	pt.AnalyzerName: {lowercase.Name, pt.LightStemmerName},
}

const defaultAnalyzer = "default_analyzer"

type BleveIndexer struct {
	fs               afero.Fs
	idx              bleve.Index
	libraryPath      string
	reader           map[string]metadata.Reader
	indexStartTime   float64
	indexedDocuments float64
}

// NewBleve creates a new BleveIndexer instance using the passed parameters
func NewBleve(index bleve.Index, fs afero.Fs, libraryPath string, read map[string]metadata.Reader) *BleveIndexer {
	return &BleveIndexer{
		fs,
		index,
		strings.TrimSuffix(libraryPath, string(filepath.Separator)),
		read,
		0,
		0,
	}
}

func Create(path string) bleve.Index {
	indexFile, err := bleve.New(path, CreateMapping())
	if err != nil {
		log.Fatal(err)
	}
	indexFile.SetInternal(internalVersion, []byte(Version))
	return indexFile
}

func CreateMapping() mapping.IndexMapping {
	indexMapping := bleve.NewIndexMapping()

	err := indexMapping.AddCustomAnalyzer(defaultAnalyzer,
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
	keywordFieldMappingNotIndexable := bleve.NewKeywordFieldMapping()
	keywordFieldMappingNotIndexable.Index = false

	simpleTextFieldMapping := bleve.NewTextFieldMapping()
	simpleTextFieldMapping.Analyzer = defaultAnalyzer

	for lang := range noStopWordsFilters {
		textFieldMapping := bleve.NewTextFieldMapping()
		textFieldMapping.Analyzer = lang

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

	indexMapping.DefaultMapping.DefaultAnalyzer = defaultAnalyzer
	indexMapping.DefaultMapping.AddFieldMappingsAt("Title", simpleTextFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Authors", simpleTextFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Description", simpleTextFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Subjects", simpleTextFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Series", simpleTextFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Slug", keywordFieldMapping)
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
