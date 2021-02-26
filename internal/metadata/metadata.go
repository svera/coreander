package metadata

import "html/template"

type Metadata struct {
	Title       string
	Author      string
	Description template.HTML
	Language    string
	Year        string
	Words       float64
	ReadingTime string
	Cover       string
}
type Reader interface {
	Metadata(file string) (Metadata, error)
	Cover(bookFullPath string, outputFolder string) error
}
