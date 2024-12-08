package index

import (
	"github.com/svera/coreander/v4/internal/metadata"
)

type Document struct {
	metadata.Metadata
	ID            string
	Slug          string
	AuthorsSlugs  []string
	SeriesSlug    string
	SubjectsSlugs []string
	Highlighted   bool
	Type          string
}

// BleveType is part of the bleve.Classifier interface and its purpose is to tell the indexer
// the type of the document, which will be used to decide which analyzer will parse it.
func (d Document) BleveType() string {
	if d.Language == "" {
		return ""
	}
	return d.Language[:2]
}
