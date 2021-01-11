package metadata

import "github.com/pirmd/epub"

func Epub(file string) (Metadata, error) {
	bk := Metadata{}
	metadata, err := epub.GetMetadataFromFile(file)
	if err != nil {
		return bk, err
	}
	title := ""
	if len(metadata.Title) > 0 {
		title = metadata.Title[0]
	}
	author := ""
	if len(metadata.Creator) > 0 {
		author = metadata.Creator[0].FullName
	}
	description := ""
	if len(metadata.Description) > 0 {
		description = metadata.Description[0]
	}
	language := ""
	if len(metadata.Language) > 0 {
		language = metadata.Language[0]
	}
	bk = Metadata{
		Title:       title,
		Author:      author,
		Description: description,
		Language:    language,
	}
	return bk, nil
}
