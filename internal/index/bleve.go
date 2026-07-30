package index

import (
	"errors"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

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
	"github.com/svera/coreander/v5/internal/metadata"
)

// DocumentVersion identifies the mapping used for indexing documents. Any changes in the mapping requires an increase
// of version, to signal that a new index needs to be created.
const DocumentVersion = "v17"

// AuthorVersion identifies the mapping used for indexing authors. Any changes in the mapping requires an increase
// of version, to signal that a new index needs to be created.
const AuthorVersion = "2"

// Metadata fields
var (
	internalLanguages          = []byte("languages")
	internalVersion            = []byte("version")
	internalIllustratedMinSize = []byte("illustrated_min_size")
)

// ErrDocumentNotFound is returned when a document cannot be found by slug.
var ErrDocumentNotFound = errors.New("document not found")

var noStopWordsFilters = map[string][]string{
	es.AnalyzerName: {lowercase.Name, es.NormalizeName, es.LightStemmerName},
	en.AnalyzerName: {lowercase.Name, en.PossessiveName, porter.Name},
	de.AnalyzerName: {lowercase.Name, de.NormalizeName, de.LightStemmerName},
	fr.AnalyzerName: {lowercase.Name, fr.ElisionName, fr.LightStemmerName},
	it.AnalyzerName: {lowercase.Name, it.ElisionName, it.LightStemmerName},
	pt.AnalyzerName: {lowercase.Name, pt.LightStemmerName},
}

const defaultAnalyzer = "default_analyzer"

// Defaults for Config.MaxSimilarityCandidates and Config.MinSimilarityScoreRatio are set here, and then
// applied by NewBleve when the caller leaves them unset (e.g. tests constructing
// a bare Config{}), since a zero value would otherwise mean "no similarity results".
const (
	// The bigger the value of defaultMaxSimilarityCandidates, the less likely a genuinely
	// similar document is cut off before MinSimilarityScoreRatio gets a chance to prune by
	// score, but the more matches Bleve has to score and rank per "similar document" query.
	defaultMaxSimilarityCandidates = 200
	// The bigger the value of defaultMinSimilarityScoreRatio, the stricter "similar enough"
	// is: a document must reach this fraction of the best match's score (0.2 = at least 20%)
	// to be shown at all, which prunes weak, mostly-coincidental matches out of the
	// maxSimilarityCandidates pool before it's paginated.
	defaultMinSimilarityScoreRatio = 0.2
)

// Config holds indexer configuration.
type Config struct {
	// IllustratedMinAmount is the minimum number of illustrations (excluding cover) for a document to be considered illustrated.
	IllustratedMinAmount int
	// IllustratedMinSize is the minimum size in megapixels for an image to count as an illustration.
	IllustratedMinSize float64
	// MinOccurrenceRatio is the minimum fraction of the most frequent
	// phrase's (or word's) occurrence count that a phrase or single word
	// must reach to be kept as a search/related-document keyword. It's the
	// indexer's call, not any particular Reader's, since it governs how
	// TextRanker.RankText results are filtered regardless of document
	// format. A value of 0 disables text ranking entirely.
	MinOccurrenceRatio float64
	// MaxSimilarityCandidates caps how many top-scoring matches a "similar
	// document" query considers before applying MinSimilarityScoreRatio and
	// paginating, so a very broad match (e.g. a common shared keyword) can't
	// force an unbounded query.
	MaxSimilarityCandidates int
	// MinSimilarityScoreRatio is the minimum fraction of the best match's
	// score a document must reach to be considered similar enough to show
	// in a "similar document" query.
	MinSimilarityScoreRatio float64
	// MaxSimilarityKeywords caps how many of a document's TextRankKeywords are
	// used, at most, to find "similar" documents. TextRankKeywords has no upper
	// bound (a long or repetitive document can end up with hundreds of
	// candidate phrases/words), and a "similar document" query ORs all of them
	// together - a wide disjunction like that is expensive to evaluate
	// regardless of how common any single keyword is, since Bleve has to poll
	// every one of those clauses for every candidate document. A value of 0
	// disables the cap.
	MaxSimilarityKeywords int
	// PreferMetadataLanguage makes TextRank trust a document's own metadata
	// language (e.g. an EPUB's declared language) directly instead of running
	// full text language detection, which is the most expensive part of
	// ranking. This misses secondary/mixed languages that full detection
	// would otherwise find (and their stop words), trading that for
	// materially faster indexing.
	PreferMetadataLanguage bool
}

type BleveIndexer struct {
	fs           afero.Fs
	documentsIdx bleve.Index // Documents index
	authorsIdx   bleve.Index // Authors index
	// documentsMu and authorsMu serialize access to documentsIdx/authorsIdx
	// respectively: AddLibrary/EnrichTextRankKeywords/RebuildAuthorsFromDocuments
	// write to these indexes from a background goroutine (see main.startIndex)
	// while the webserver concurrently searches them, and without this guard
	// concurrent Batch/Search calls have been observed to panic inside
	// bleve/zapx (an out-of-range read while decoding a segment being merged).
	documentsMu                sync.RWMutex
	authorsMu                  sync.RWMutex
	libraryPath                string
	reader                     map[string]metadata.Reader
	indexStartNanos            atomic.Int64
	indexedEntries             atomic.Uint64
	indexTotalEntries          atomic.Uint64
	authorEnrichStartNanos     atomic.Int64
	authorEnrichProcessed      atomic.Uint64
	authorEnrichTotalEntries   atomic.Uint64
	textRankEnrichStartNanos   atomic.Int64
	textRankEnrichProcessed    atomic.Uint64
	textRankEnrichTotalEntries atomic.Uint64
	illustratedMinAmount       int     // minimum number of illustrations (excl. cover) for a document to be considered illustrated
	illustratedMinSize         float64 // minimum size in megapixels for an image to count as an illustration
	minOccurrenceRatio         float64 // minimum occurrence ratio for a TextRank phrase/word to be kept; see Config.MinOccurrenceRatio
	maxSimilarityCandidates    int     // cap on top-scoring matches considered by a "similar document" query; see Config.MaxSimilarityCandidates
	minSimilarityScoreRatio    float64 // minimum fraction of the best match's score to be considered similar; see Config.MinSimilarityScoreRatio
	maxSimilarityKeywords      int     // cap on how many TextRankKeywords are used to find "similar" documents; see Config.MaxSimilarityKeywords
	preferMetadataLanguage     bool    // trust a document's own metadata language over full text detection for TextRank; see Config.PreferMetadataLanguage
}

// NewBleve creates a new BleveIndexer instance using the passed parameters
func NewBleve(documentsIndex bleve.Index, authorsIndex bleve.Index, fs afero.Fs, libraryPath string, read map[string]metadata.Reader, cfg Config) *BleveIndexer {
	maxSimilarityCandidates := cfg.MaxSimilarityCandidates
	if maxSimilarityCandidates == 0 {
		maxSimilarityCandidates = defaultMaxSimilarityCandidates
	}

	minSimilarityScoreRatio := cfg.MinSimilarityScoreRatio
	if minSimilarityScoreRatio == 0 {
		minSimilarityScoreRatio = defaultMinSimilarityScoreRatio
	}

	// Unlike MaxSimilarityCandidates/MinSimilarityScoreRatio above, cfg.MaxSimilarityKeywords
	// is passed straight through with no zero-substitution: 0 is a legitimate, documented
	// choice here (disable the cap - see Config.MaxSimilarityKeywords), the same as
	// Config.MinOccurrenceRatio's "0 disables this" elsewhere in this Config. The normal
	// (production) case still gets defaultMaxSimilarityKeywords via the CLI flag's own
	// default, not a substitution here; only callers that construct a bare Config{}
	// directly (e.g. tests) get 0 (uncapped) rather than defaultMaxSimilarityKeywords.
	maxSimilarityKeywords := cfg.MaxSimilarityKeywords

	return &BleveIndexer{
		fs:                      fs,
		documentsIdx:            documentsIndex,
		authorsIdx:              authorsIndex,
		libraryPath:             strings.TrimSuffix(libraryPath, string(filepath.Separator)),
		reader:                  read,
		illustratedMinAmount:    cfg.IllustratedMinAmount,
		illustratedMinSize:      cfg.IllustratedMinSize,
		minOccurrenceRatio:      cfg.MinOccurrenceRatio,
		maxSimilarityCandidates: maxSimilarityCandidates,
		minSimilarityScoreRatio: minSimilarityScoreRatio,
		maxSimilarityKeywords:   maxSimilarityKeywords,
		preferMetadataLanguage:  cfg.PreferMetadataLanguage,
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

	simpleTextFieldMapping := bleve.NewTextFieldMapping()
	simpleTextFieldMapping.Analyzer = defaultAnalyzer
	simpleTextFieldMapping.Similarity = index.BM25Scoring

	numericFieldMapping := bleve.NewNumericFieldMapping()
	dateTimeFieldMapping := bleve.NewDateTimeFieldMapping()
	booleanFieldMapping := bleve.NewBooleanFieldMapping()

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
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("IllustratorsSlugs", keywordFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Illustrators", simpleTextFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Description", textFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("TextRankKeywords", textFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Subjects", keywordFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("SubjectsSlugs", keywordFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Series", noStopWordsTextFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("SeriesSlug", keywordFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Language", keywordFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Publication.Date", numericFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Publication.Precision", numericFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Words", numericFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Pages", numericFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Illustrations", numericFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("AddedOn", dateTimeFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("TextRankEnriched", booleanFieldMapping)
	}

	indexMapping.DefaultMapping.DefaultAnalyzer = defaultAnalyzer
	indexMapping.DefaultMapping.AddFieldMappingsAt("Slug", keywordFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Title", simpleTextFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Authors", simpleTextFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("AuthorsSlugs", keywordFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("IllustratorsSlugs", keywordFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Illustrators", simpleTextFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Description", simpleTextFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("TextRankKeywords", simpleTextFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Subjects", keywordFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("SubjectsSlugs", keywordFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Series", simpleTextFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("SeriesSlug", keywordFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Language", keywordFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Publication.Date", numericFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Publication.Precision", numericFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Words", numericFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Pages", numericFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Illustrations", numericFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("AddedOn", dateTimeFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("TextRankEnriched", booleanFieldMapping)

	return indexMapping
}

func CreateAuthorsMapping() mapping.IndexMapping {
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

	numericFieldMapping := bleve.NewNumericFieldMapping()
	dateTimeFieldMapping := bleve.NewDateTimeFieldMapping()

	indexMapping.DefaultMapping.DefaultAnalyzer = defaultAnalyzer
	indexMapping.DefaultMapping.AddFieldMappingsAt("Slug", keywordFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("Name", simpleTextFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("BirthName", simpleTextFieldMapping)
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
	indexMapping.DefaultMapping.AddFieldMappingsAt("DocumentCount", numericFieldMapping)

	return indexMapping
}

// Close closes both indexes
func (b *BleveIndexer) Close() error {
	b.documentsMu.Lock()
	defer b.documentsMu.Unlock()
	b.authorsMu.Lock()
	defer b.authorsMu.Unlock()
	return errors.Join(b.documentsIdx.Close(), b.authorsIdx.Close())
}

// NeedsReindexForIllustratedConfig reports whether the documents index must be rebuilt because the stored
// illustrated-min-size config differs from currentMinSize (or is missing).
func NeedsReindexForIllustratedConfig(documentsIndex bleve.Index, currentMinSize float64) (bool, error) {
	stored, err := documentsIndex.GetInternal(internalIllustratedMinSize)
	if err != nil {
		return true, err
	}
	if len(stored) == 0 {
		return true, nil
	}
	storedSize, err := strconv.ParseFloat(string(stored), 64)
	if err != nil {
		return true, err
	}
	return storedSize != currentMinSize, nil
}
