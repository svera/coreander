package metadata

import "github.com/pirmd/epub"

// FromEpubInformation builds Metadata from pirmd/epub Information without reading the file.
// Illustration and word counts are not populated; use EpubReader.Metadata for a full read.
func FromEpubInformation(filename string, info *epub.Information) (Metadata, error) {
	return buildEpubMetadataFields(info, filename)
}
