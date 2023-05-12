package metadata

import (
	"html/template"
)

type Metadata struct {
	ID          string
	Title       string
	Authors     []string
	Description template.HTML
	Language    string
	Year        string
	Words       float64
	ReadingTime string
	Cover       string
	Series      string
	SeriesIndex float64
	Pages       int
	Type        string
}
type Reader interface {
	Metadata(file string) (Metadata, error)
	Cover(bookFullPath string, coverMaxWidth int) ([]byte, error)
}
