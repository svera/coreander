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
	index "github.com/blevesearch/bleve_index_api"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/metadata"
)

// DocumentVersion identifies the mapping used for indexing documents. Any changes in the mapping requires an increase
// of version, to signal that a new index needs to be created.
const DocumentVersion = "v9"

// AuthorVersion identifies the mapping used for indexing authors. Any changes in the mapping requires an increase
// of version, to signal that a new index needs to be created.
const AuthorVersion = "1"

// Metadata fields
var (
	internalLanguages = []byte("languages")
	internalVersion   = []byte("version")
)

var noStopWordsFilters = map[string][]string{
	es.AnalyzerName: {lowercase.Name, es.NormalizeName, es.LightStemmerName},
	en.AnalyzerName: {lowercase.Name, en.PossessiveName, porter.Name},
	de.AnalyzerName: {lowercase.Name, de.NormalizeName, de.LightStemmerName},
	fr.AnalyzerName: {lowercase.Name, fr.ElisionName, fr.LightStemmerName},
	it.AnalyzerName: {lowercase.Name, it.ElisionName, it.LightStemmerName},
	pt.AnalyzerName: {lowercase.Name, pt.LightStemmerName},
}

const defaultAnalyzer = "default_analyzer"

type BleveIndexer struct {
	fs             afero.Fs
	idx            bleve.Index // Documents index
	authorsIdx     bleve.Index // Authors index
	libraryPath    string
	reader         map[string]metadata.Reader
	indexStartTime float64
	indexedEntries float64
}

// NewBleve creates a new BleveIndexer instance using the passed parameters
func NewBleve(documentsIndex bleve.Index, authorsIndex bleve.Index, fs afero.Fs, libraryPath string, read map[string]metadata.Reader) *BleveIndexer {
	return &BleveIndexer{
		fs:             fs,
		idx:            documentsIndex,
		authorsIdx:     authorsIndex,
		libraryPath:    strings.TrimSuffix(libraryPath, string(filepath.Separator)),
		reader:         read,
		indexStartTime: 0,
		indexedEntries: 0,
	}
}

func CreateDocumentsIndex(path string) bleve.Index {
	indexFile, err := bleve.New(path, CreateDocumentsMapping())
	if err != nil {
		log.Fatal(err)
	}
	indexFile.SetInternal(internalVersion, []byte(DocumentVersion))
	return indexFile
}

func CreateAuthorsIndex(path string) bleve.Index {
	indexFile, err := bleve.New(path, CreateAuthorsMapping())
	if err != nil {
		log.Fatal(err)
	}
	indexFile.SetInternal(internalVersion, []byte(AuthorVersion))
	return indexFile
}

func CreateDocumentsMapping() mapping.IndexMapping {
	indexMapping := bleve.NewIndexMapping()

	err := indexMapping.AddCustomAnalyzer(defaultAnalyzer,
		map[string]any{
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
	simpleTextFieldMapping.Similarity = index.BM25Scoring

	numericFieldMapping := bleve.NewNumericFieldMapping()
	dateTimeFieldMapping := bleve.NewDateTimeFieldMapping()

	for lang := range noStopWordsFilters {
		textFieldMapping := bleve.NewTextFieldMapping()
		textFieldMapping.Analyzer = lang
		textFieldMapping.Similarity = index.BM25Scoring

		err := addNoStopWordsAnalyzer(lang, indexMapping)
		if err != nil {
			log.Fatal(err)
		}
		noStopWordsTextFieldMapping := bleve.NewTextFieldMapping()
		noStopWordsTextFieldMapping.Analyzer = lang + "_no_stop_words"
		noStopWordsTextFieldMapping.Similarity = index.BM25Scoring

		indexMapping.AddDocumentMapping(lang, bleve.NewDocumentMapping())
		indexMapping.TypeMapping[lang].DefaultAnalyzer = lang
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Slug", keywordFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Title", noStopWordsTextFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Authors", simpleTextFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("AuthorsSlugs", keywordFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Description", textFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Subjects", textFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("SubjectsSlugs", keywordFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Series", noStopWordsTextFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("SeriesSlug", keywordFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Language", keywordFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Publication.Date", numericFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Publication.Precision", numericFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Words", numericFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Pages", numericFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("AddedOn", dateTimeFieldMapping)
	}

	indexMapping.DefaultMapping.DefaultAnalyzer = defaultAnalyzer
	indexMapping.DefaultMapping.AddFieldMappingsAt("Slug", keywordFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Title", simpleTextFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Authors", simpleTextFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("AuthorsSlugs", keywordFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Description", simpleTextFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Subjects", simpleTextFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("SubjectsSlugs", keywordFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Series", simpleTextFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("SeriesSlug", keywordFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Language", keywordFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Publication.Date", numericFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Publication.Precision", numericFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Words", numericFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Pages", numericFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("AddedOn", dateTimeFieldMapping)

	return indexMapping
}

func CreateAuthorsMapping() mapping.IndexMapping {
	indexMapping := bleve.NewIndexMapping()

	keywordFieldMapping := bleve.NewKeywordFieldMapping()
	keywordFieldMappingNotIndexable := bleve.NewKeywordFieldMapping()
	keywordFieldMappingNotIndexable.Index = false

	numericFieldMapping := bleve.NewNumericFieldMapping()
	dateTimeFieldMapping := bleve.NewDateTimeFieldMapping()

	indexMapping.DefaultMapping.AddFieldMappingsAt("Slug", keywordFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Name", keywordFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("BirthName", keywordFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("RetrievedOn", dateTimeFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("DataSourceID", keywordFieldMappingNotIndexable)
	indexMapping.DefaultMapping.AddFieldMappingsAt("DataSourceImage", keywordFieldMappingNotIndexable)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Website", keywordFieldMappingNotIndexable)
	indexMapping.DefaultMapping.AddFieldMappingsAt("DateOfBirth.Date", numericFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("DateOfBirth.Precision", numericFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("DateOfDeath.Date", numericFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("DateOfDeath.Precision", numericFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("InstanceOf", numericFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Gender", numericFieldMapping)

	return indexMapping
}

// Close closes both indexes
func (b *BleveIndexer) Close() error {
	if err := b.idx.Close(); err != nil {
		return err
	}
	return b.authorsIdx.Close()
}
