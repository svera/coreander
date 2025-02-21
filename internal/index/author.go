package index

type Author struct {
	Slug       string
	Name       string
	WikidataID string
	Type       string
}

// BleveType is part of the bleve.Classifier interface and its purpose is to tell the indexer
// the type of the document, which will be used to decide which analyzer will parse it.
func (a Author) BleveType() string {
	return "author"
}

const (
	InstanceUnknown = iota
	InstanceHuman
	InstancePseudonym
	InstancePenName
	InstanceCollectivePseudonym
)

const (
	GenderUnknown = iota
	GenderMale
	GenderFemale
	GenderIntersex
	GenderTrasgenderFemale
	GenderTrasgenderMale
)
