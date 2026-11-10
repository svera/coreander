package index

import (
	"encoding/json"
	"fmt"
	"maps"
	"time"

	"github.com/rickb777/date/v2"
	"github.com/svera/coreander/v5/internal/metadata"
)

// DataSourceScoreValueSortField is the index sort key for the configured data-source score.
const DataSourceScoreValueSortField = "DataSourceScoreValue"

type SearchFields struct {
	Keywords string
	// AuthorSlug, when set, scopes the search to documents by (or illustrated
	// by) this author, see SearchByAuthor.
	AuthorSlug string
	Language   string
	Subjects   string
	// SimilarTo, when set to a document slug, ignores Keywords/Subjects and
	// instead returns documents sharing the configured minimum of uncommon TextRank keyword
	// phrases (via similarToQuery, excluding the document itself, its own
	// author and its own series). When both books have subjects known to be uncommon
	// in the library, they must share at least one of those subjects.
	// Results are ranked by score and pruned to those scoring close
	// enough to the best match (see runSimilarityQuery).
	SimilarTo           string
	PubDateFrom         date.Date
	PubDateTo           date.Date
	EstReadTimeFrom     float64
	EstReadTimeTo       float64
	WordsPerMinute      float64
	PagesFrom           float64
	PagesTo             float64
	DataSourceScoreFrom float64
	DataSourceScoreTo   *float64
	IllustratedOnly     bool
	SortBy              []string
}

// DataSourceScoreUpperBound defaults an unspecified upper bound to the maximum score.
func (s SearchFields) DataSourceScoreUpperBound() float64 {
	if s.DataSourceScoreTo == nil {
		return 5
	}
	return *s.DataSourceScoreTo
}

type Document struct {
	metadata.Metadata
	ID                string
	Slug              string
	AuthorsSlugs      []string
	IllustratorsSlugs []string
	SeriesSlug        string
	SubjectsSlugs     []string
	AddedOn           time.Time
	// TextRankPhrases holds the word pairs (two-word phrases, e.g. "robert
	// oppenheimer") extracted by TextRank analysis at indexing time (EPUB
	// only), one phrase per element, ordered by descending TextRank weight
	// (see textRankKeywords) - callers that only use a prefix of this slice
	// (e.g. similarToQuery's Config.MaxSimilarityPhrases cap) get the most
	// representative phrases first rather than an arbitrary subset. Storing
	// one entry per phrase (rather than flattening them all into a single
	// string) matters: Bleve resets term positions at each array element, so
	// a MatchPhraseQuery against this field only ever matches an actual
	// adjacent pair, never two words from different, unrelated pairs that
	// just ended up next to each other after flattening. Indexed with
	// defaultAnalyzer (no stemming) rather than a per-language analyzer,
	// deliberately: MatchPhraseQuery needs index-time and query-time
	// tokenization to line up exactly, which stemming would break (see
	// CreateDocumentsMapping).
	TextRankPhrases []string
	// TextRankWords holds the single words extracted by TextRank analysis at
	// indexing time (EPUB only), ordered by descending TextRank weight (see
	// textRankKeywords). Unlike TextRankPhrases, this field is indexed with
	// the document's own per-language analyzer (stemming), since it's only
	// ever matched with MatchQuery (composeQuery's general keyword search),
	// which has no adjacency requirement to protect - stemming here instead
	// lets a search for a singular/plural form of a word match a keyword
	// stored in the other form. This field is analyzed and indexed so a
	// document can be found by its key topics even when they don't appear in
	// Title/Authors/Description/Subjects.
	TextRankWords []string
	// TextRankEnriched is false until EnrichTextRankKeywords has processed
	// this document (or immediately true at indexing time for formats that
	// can never support it, e.g. PDF). AddLibrary intentionally skips TextRank
	// analysis so the library becomes searchable as fast as possible;
	// EnrichTextRankKeywords fills TextRankPhrases/TextRankWords in afterward,
	// in the background. See documentsNeedingTextRank, which drives that pass
	// off this field so it survives (and resumes correctly across) restarts.
	TextRankEnriched bool
	// DataSources holds opaque provider-owned JSON objects, stored but not indexed.
	DataSources map[string]json.RawMessage `json:"-"`
	// DataSourceScoreValue is the configured data source's score, stored and indexed for filtering and sorting.
	DataSourceScoreValue float64
}

// DataSourceData decodes a provider's payload. Missing providers have zero-valued data.
func DataSourceData[T any](document Document, source string) (T, error) {
	var data T
	if raw, ok := document.DataSources[source]; ok {
		if err := json.Unmarshal(raw, &data); err != nil {
			return data, fmt.Errorf("decoding %s data for document %s: %w", source, document.ID, err)
		}
	}
	return data, nil
}

// SetDataSourceData replaces one provider's payload without mutating shared document maps.
func SetDataSourceData(document *Document, source string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("encoding %s data for document %s: %w", source, document.ID, err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return fmt.Errorf("data for provider %s must be a JSON object", source)
	}
	document.DataSources = maps.Clone(document.DataSources)
	if document.DataSources == nil {
		document.DataSources = make(map[string]json.RawMessage)
	}
	document.DataSources[source] = raw
	return nil
}

// IndexDocument stores provider JSON without exposing its contents to Bleve's dynamic mapping.
func IndexDocument(destination interface{ Index(string, any) error }, id string, document Document) error {
	for source, raw := range document.DataSources {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(raw, &object); err != nil || object == nil {
			return fmt.Errorf("invalid JSON object for provider %s in document %s", source, id)
		}
	}
	raw, err := json.Marshal(document.DataSources)
	if err != nil {
		return fmt.Errorf("encoding data sources for document %s: %w", id, err)
	}
	var enriched []string
	for source, payload := range document.DataSources {
		var state struct {
			Enriched bool `json:"enriched"`
		}
		if err := json.Unmarshal(payload, &state); err != nil {
			return fmt.Errorf("reading enrichment state for provider %s in document %s: %w", source, id, err)
		}
		if state.Enriched {
			enriched = append(enriched, source)
		}
	}
	return destination.Index(id, struct {
		Document
		DataSources         string
		EnrichedDataSources []string
	}{document, string(raw), enriched})
}

func (d Document) DataSourceScore() float64 {
	return d.DataSourceScoreValue
}

func (d Document) SupportsAnnotations() bool {
	return d.Format == "EPUB"
}

// BleveType is part of the bleve.Classifier interface and its purpose is to tell the indexer
// the type of the document, which will be used to decide which analyzer will parse it.
func (d Document) BleveType() string {
	if d.Language == "" {
		return ""
	}
	return d.Language[:2]
}
