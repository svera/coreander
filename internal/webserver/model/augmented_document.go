package model

import (
	"time"

	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/result"
)

type AugmentedDocument struct {
	index.Document
	Highlight              Highlight
	CompletedOn            *time.Time
	ReadingProgressPercent int // 0–100 from readings.progress when set; used on home resume block
}

func AugmentedDocumentsFromDocuments(results result.Paginated[[]index.Document]) result.Paginated[[]AugmentedDocument] {
	documents := make([]AugmentedDocument, len(results.Hits()))
	for i, doc := range results.Hits() {
		documents[i] = AugmentedDocument{
			Document:  doc,
			Highlight: Highlight{},
		}
	}

	return result.NewPaginated(
		ResultsPerPage,
		results.Page(),
		results.TotalHits(),
		documents,
	)
}
