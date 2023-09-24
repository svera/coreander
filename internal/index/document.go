package index

import "github.com/svera/coreander/v3/internal/metadata"

type Document struct {
	metadata.Metadata
	ID        string
	BaseName  string
	Slug      string
	AuthorsEq []string
	SeriesEq  string
}
