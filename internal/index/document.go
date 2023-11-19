package index

import "github.com/svera/coreander/v4/internal/metadata"

type Document struct {
	metadata.Metadata
	ID          string
	Slug        string
	Highlighted bool
}

// DocumentWrite is an extension to Document that is used only when writing to the index,
// as some of its fields are only used to perform searches and not returned
type DocumentWrite struct {
	Document
	AuthorsEq  []string
	SeriesEq   string
	SubjectsEq []string
}
