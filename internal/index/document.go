package index

import (
	"time"

	"github.com/rickb777/date/v2"
	"github.com/svera/coreander/v5/internal/metadata"
)

type SearchFields struct {
	Keywords string
	Language string
	Subjects string
	// SimilarTo, when set to a document slug, ignores Keywords/Subjects and
	// instead returns documents matching that document's subjects/TextRank
	// keywords (via the same subjectsQuery used by SameSubjects, excluding
	// the document itself, its own author and its own series), ranked by
	// score and pruned to those scoring close enough to the best match
	// (see runSimilarityQuery). Powers the "See all" link on a document's
	// "With similar subjects" section.
	SimilarTo       string
	PubDateFrom     date.Date
	PubDateTo       date.Date
	EstReadTimeFrom float64
	EstReadTimeTo   float64
	WordsPerMinute  float64
	PagesFrom       float64
	PagesTo         float64
	IllustratedOnly bool
	SortBy          []string
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
	// TextRankKeywords holds the word pairs (two-word phrases, e.g. "robert
	// oppenheimer") and single words extracted by TextRank analysis at
	// indexing time (EPUB only), one phrase/word per element, ordered by
	// descending TextRank weight (see textRankKeywords) - callers that only
	// use a prefix of this slice (e.g. subjectsQuery's
	// Config.MaxSimilarityKeywords cap) get the most representative keywords
	// first rather than an arbitrary subset. Storing one entry per
	// phrase/word (rather than flattening them all into a single string)
	// matters: Bleve resets term positions at each array element, so a
	// MatchPhraseQuery against this field only ever matches an actual
	// adjacent pair, never two words from different, unrelated pairs that
	// just ended up next to each other after flattening. This field is
	// analyzed and indexed so a document can be found by its key topics even
	// when they don't appear in Title/Authors/Description/Subjects.
	TextRankKeywords []string
	// TextRankEnriched is false until EnrichTextRankKeywords has processed
	// this document (or immediately true at indexing time for formats that
	// can never support it, e.g. PDF). AddLibrary intentionally skips TextRank
	// analysis so the library becomes searchable as fast as possible;
	// EnrichTextRankKeywords fills TextRankKeywords in afterward, in the
	// background. See documentsNeedingTextRank, which drives that pass off
	// this field so it survives (and resumes correctly across) restarts.
	TextRankEnriched bool
}

// BleveType is part of the bleve.Classifier interface and its purpose is to tell the indexer
// the type of the document, which will be used to decide which analyzer will parse it.
func (d Document) BleveType() string {
	if d.Language == "" {
		return ""
	}
	return d.Language[:2]
}
