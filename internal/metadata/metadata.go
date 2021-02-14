package metadata

type Metadata struct {
	Title       string
	Author      string
	Description string
	Language    string
	Year        string
	Words       float64
	ReadingTime string
}

// Type is a method used by bleve to know which analyzer use with a document
func (b Metadata) Type() string {
	return "book"
}

type Reader func(file string) (Metadata, error)
