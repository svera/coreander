package model

import (
	"time"

	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/result"
)

type SearchResult struct {
	Document    index.Document
	Highlight   Highlight
	CompletedOn *time.Time
}

func SearchResultsFromDocuments(results result.Paginated[[]index.Document]) result.Paginated[[]SearchResult] {
	searchResults := make([]SearchResult, len(results.Hits()))
	for i, doc := range results.Hits() {
		searchResults[i] = SearchResult{
			Document: doc,
			Highlight: Highlight{},
		}
	}

	return result.NewPaginated(
		ResultsPerPage,
		results.Page(),
		results.TotalHits(),
		searchResults,
	)
}
