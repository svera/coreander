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
	// instead returns documents related to that document exactly as SameSubjects
	// would (shared subjects or TextRank keywords, excluding the document
	// itself, its own author and its own series) - powers the "See all" link
	// on a document's "With similar subjects" section, so results match.
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
	// TextRankKeywords holds the phrases/words extracted by TextRank analysis
	// at indexing time (EPUB only) as plain, space-separated text, analyzed
	// and indexed so a document can be found by its key topics/phrases even
	// when they don't appear in Title/Authors/Description/Subjects.
	TextRankKeywords string
}

// BleveType is part of the bleve.Classifier interface and its purpose is to tell the indexer
// the type of the document, which will be used to decide which analyzer will parse it.
func (d Document) BleveType() string {
	if d.Language == "" {
		return ""
	}
	return d.Language[:2]
}
