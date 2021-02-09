package metadata

type Metadata struct {
	Title       string
	Author      string
	Description string
	Language    string
	Year        string
}

// Type is a method used by bleve to know which analyzer use with a document
func (b Metadata) Type() string {
	return b.Language
}

type Reader func(file string) (Metadata, error)
