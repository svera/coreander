package index

import "github.com/svera/coreander/v3/internal/metadata"

type Document struct {
	metadata.Metadata
	ID          string
	Slug        string
	Highlighted bool
}

type Strict struct {
	Value  string
	Length float64
}

// DocumentWrite is an extension to Document that is used only when writing to the index,
// as some of its fields are only used to perform searches and not returned
type DocumentWrite struct {
	Document
	AuthorsEq  []Strict
	SeriesEq   Strict
	SubjectsEq []Strict
}

// BleveType is part of the bleve.Classifier interface and its purpose is to tell the indexer
// the type of the document, which will be used to decide which analyzer will parse it.
func (d DocumentWrite) BleveType() string {
	if d.Language == "" {
		return ""
	}
	return d.Language[:2]
}
