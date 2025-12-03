package index

import (
	"time"

	"github.com/rickb777/date/v2"
	"github.com/svera/coreander/v4/internal/metadata"
)

type SearchFields struct {
	Keywords        string
	Language        string
	PubDateFrom     date.Date
	PubDateTo       date.Date
	EstReadTimeFrom float64
	EstReadTimeTo   float64
	WordsPerMinute  float64
	SortBy          []string
}

type Document struct {
	metadata.Metadata
	ID            string
	Slug          string
	AuthorsSlugs  []string
	SeriesSlug    string
	SubjectsSlugs []string
	Highlighted   bool
	CompletedOn   *time.Time
	AddedOn       time.Time
}
