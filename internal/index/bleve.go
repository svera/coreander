package index

import (
	"errors"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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
const DocumentVersion = "v13"

// AuthorVersion identifies the mapping used for indexing authors. Any changes in the mapping requires an increase
// of version, to signal that a new index needs to be created.
const AuthorVersion = "2"

// Metadata fields
var (
	internalLanguages          = []byte("languages")
	internalVersion            = []byte("version")
	internalIllustratedMinSize = []byte("illustrated_min_size")
	internalMinOccurrenceRatio = []byte("min_occurrence_ratio")
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
	defaultMinSimilarityScoreRatio = 0.3
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
	// MaxSimilarityPhrases caps how many of a document's TextRankPhrases are
	// used, at most, to find "similar" documents. TextRankPhrases has no upper
	// bound (a long or repetitive document can end up with hundreds of
	// candidate phrases), and a "similar document" query ORs all of them
	// together - a wide disjunction like that is expensive to evaluate
	// regardless of how common any single phrase is, since Bleve has to poll
	// every one of those clauses for every candidate document. A value of 0
	// disables the cap.
	MaxSimilarityPhrases int
	// MaxTextRankWords caps how many words of a document's extracted text
	// TextRank analysis considers: only the first MaxTextRankWords words are
	// analyzed for a document whose text exceeds this, so its TextRankPhrases
	// /TextRankWords end up based on its opening portion rather than the
	// whole text (Document.Words is unaffected, still counting the whole
	// document). The underlying TextRank library builds an in-memory graph
	// (word connections, per-word-pair sentence lists, a second copy of
	// every sentence) that grows with every word occurrence, not just
	// distinct words, so a very long or repetitive document (an omnibus, a
	// badly-OCR'd scan) can use several hundred MB of RAM for a single
	// document, single-threaded - enough to trip the OOM killer on a small
	// VM/container regardless of worker count. A value of 0 disables the cap
	// (analyze documents of any size in full).
	MaxTextRankWords int
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
	documentsMu             sync.RWMutex
	authorsMu               sync.RWMutex
	libraryPath             string
	reader                  map[string]metadata.Reader
	indexProgress           progressTracker
	authorEnrichProgress    progressTracker
	textRankEnrichProgress  progressTracker
	illustratedMinAmount    int     // minimum number of illustrations (excl. cover) for a document to be considered illustrated
	illustratedMinSize      float64 // minimum size in megapixels for an image to count as an illustration
	minOccurrenceRatio      float64 // minimum occurrence ratio for a TextRank phrase/word to be kept; see Config.MinOccurrenceRatio
	maxSimilarityCandidates int     // cap on top-scoring matches considered by a "similar document" query; see Config.MaxSimilarityCandidates
	minSimilarityScoreRatio float64 // minimum fraction of the best match's score to be considered similar; see Config.MinSimilarityScoreRatio
	maxSimilarityPhrases    int     // cap on how many TextRankPhrases are used to find "similar" documents; see Config.MaxSimilarityPhrases
	maxTextRankWords        int     // word count above which TextRank analysis only considers a document's first maxTextRankWords words; see Config.MaxTextRankWords
}

// progressTracker holds the atomic counters behind one phase of
// IndexingProgress (indexing, author enrichment, or TextRank enrichment):
// a start time, how many entries have been processed, and the total
// expected. Zero value means "not in progress".
type progressTracker struct {
	startNanos atomic.Int64
	processed  atomic.Uint64
	total      atomic.Uint64
}

// begin marks the phase as started, resetting processed to 0 and total to
// the given count.
func (p *progressTracker) begin(total int) {
	p.startNanos.Store(time.Now().UnixNano())
	p.processed.Store(0)
	p.total.Store(uint64(total))
}

// end marks the phase as no longer in progress.
func (p *progressTracker) end() {
	p.startNanos.Store(0)
	p.processed.Store(0)
	p.total.Store(0)
}

// record increments the number of entries processed so far by one.
func (p *progressTracker) record() {
	p.processed.Add(1)
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

	// Unlike MaxSimilarityCandidates/MinSimilarityScoreRatio above, cfg.MaxSimilarityPhrases
	// is passed straight through with no zero-substitution: 0 is a legitimate, documented
	// choice here (disable the cap - see Config.MaxSimilarityPhrases), the same as
	// Config.MinOccurrenceRatio's "0 disables this" elsewhere in this Config. The normal
	// (production) case still gets defaultMaxSimilarityPhrases via the CLI flag's own
	// default, not a substitution here; only callers that construct a bare Config{}
	// directly (e.g. tests) get 0 (uncapped) rather than defaultMaxSimilarityPhrases.
	maxSimilarityPhrases := cfg.MaxSimilarityPhrases

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
		maxSimilarityPhrases:    maxSimilarityPhrases,
		maxTextRankWords:        cfg.MaxTextRankWords,
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
	// BM25 saturates term frequency and normalizes by how a field's length
	// compares to the index's average, rather than classic TF-IDF's raw
	// 1/sqrt(fieldLength) norm - which systematically penalizes documents
	// with a longer TextRankPhrases/TextRankWords list (e.g. a book yielding many
	// TextRank phrases) relative to shorter ones, regardless of relevance. This is set
	// at the index level because bleve only supports choosing the scoring
	// model here, via IndexMappingImpl.ScoringModel - per-field
	// FieldMapping.Similarity (see below) is for KNN vector fields only and
	// has no effect on text scoring.
	indexMapping.ScoringModel = index.BM25Scoring

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

	numericFieldMapping := bleve.NewNumericFieldMapping()
	dateTimeFieldMapping := bleve.NewDateTimeFieldMapping()
	booleanFieldMapping := bleve.NewBooleanFieldMapping()

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
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Slug", keywordFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Title", noStopWordsTextFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Authors", simpleTextFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("AuthorsSlugs", keywordFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("IllustratorsSlugs", keywordFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Illustrators", simpleTextFieldMapping)
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("Description", textFieldMapping)
		// TextRankPhrases is compared via exact-phrase MatchPhraseQuery in
		// subjectsQuery (see bleve_document_read.go), which analyzes its query
		// terms with defaultAnalyzer (no stemming). Mapping this field to the
		// per-language textFieldMapping here - which does stem, e.g. Spanish's
		// light stemmer - would silently break most phrase matches: the query
		// tokenizes "occidental" as "occidental", but the index may have
		// stored a stemmed form, so tokens never line up. Since this field
		// exists specifically to detect exact recurring phrase reuse across
		// documents (not fuzzy keyword search), it should never stem in the
		// first place - simpleTextFieldMapping (defaultAnalyzer) keeps
		// index-time and query-time tokenization identical for every
		// document, regardless of its language.
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("TextRankPhrases", simpleTextFieldMapping)
		// TextRankWords, unlike TextRankPhrases, is only ever matched with a
		// plain MatchQuery (composeQuery's general keyword search), which has
		// no adjacency to protect - so it's mapped to the per-language
		// textFieldMapping (stemming) like Description, letting a search for
		// "potencia" match a stored "potencias" and vice versa.
		indexMapping.TypeMapping[lang].AddFieldMappingsAt("TextRankWords", textFieldMapping)
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
	indexMapping.DefaultMapping.AddFieldMappingsAt("TextRankPhrases", simpleTextFieldMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("TextRankWords", simpleTextFieldMapping)
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

// NeedsReindex reports whether the documents index must be rebuilt because a stored config value
// differs from its current counterpart (or is missing): illustrated-min-size, or min-occurrence-ratio,
// which decides which TextRank keywords get stored per document at indexing time (see
// Config.MinOccurrenceRatio).
func NeedsReindex(documentsIndex bleve.Index, currentMinSize float64, currentMinOccurrenceRatio float64) (bool, error) {
	storedMinSize, err := documentsIndex.GetInternal(internalIllustratedMinSize)
	if err != nil {
		return true, err
	}
	if len(storedMinSize) == 0 {
		return true, nil
	}
	minSize, err := strconv.ParseFloat(string(storedMinSize), 64)
	if err != nil {
		return true, err
	}
	if minSize != currentMinSize {
		return true, nil
	}

	storedRatio, err := documentsIndex.GetInternal(internalMinOccurrenceRatio)
	if err != nil {
		return true, err
	}
	if len(storedRatio) == 0 {
		return true, nil
	}
	ratio, err := strconv.ParseFloat(string(storedRatio), 64)
	if err != nil {
		return true, err
	}
	return ratio != currentMinOccurrenceRatio, nil
}
