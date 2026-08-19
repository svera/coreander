package index_test

import (
	"html/template"
	"image"
	"path/filepath"
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/metadata"
)

// duplicateTitleReader always reports the same title and author regardless of
// the file being read, simulating two distinct files in the library that
// happen to share title and author (e.g. two different editions of Ivanhoe).
type duplicateTitleReader struct{}

func (duplicateTitleReader) Metadata(file string) (metadata.Metadata, error) {
	return metadata.Metadata{
		Title:       "Ivanhoe",
		Authors:     []string{"Walter Scott"},
		Description: template.HTML("<p>same title and author</p>"),
		Language:    "en",
		Format:      "EPUB",
		Words:       1000,
	}, nil
}

func (duplicateTitleReader) Cover(string, int) (image.Image, error) {
	return nil, nil
}

// TestAddLibraryAssignsUniqueSlugsToDuplicateTitles ensures that two distinct
// files sharing the same title and author end up with different slugs, even
// when they land in separate indexing batches (batchSize 1 forces each file
// into its own chunk/commit).
func TestAddLibraryAssignsUniqueSlugsToDuplicateTitles(t *testing.T) {
	fs := afero.NewMemMapFs()
	const lib = "lib"
	if err := fs.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ivanhoe1.epub", "ivanhoe2.epub"} {
		if err := afero.WriteFile(fs, filepath.Join(lib, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	docIdx, err := bleve.NewMemOnly(index.CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}
	authIdx, err := bleve.NewMemOnly(index.CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}
	readers := map[string]metadata.Reader{".epub": duplicateTitleReader{}}
	idx := index.NewBleve(docIdx, authIdx, fs, lib, readers, index.Config{})
	defer idx.Close()

	if err := idx.AddLibrary(1, true, 1); err != nil {
		t.Fatalf("AddLibrary: %v", err)
	}

	results, err := idx.Search(index.SearchFields{Keywords: "Ivanhoe"}, 1, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	hits := results.Hits()
	if len(hits) != 2 {
		t.Fatalf("expected 2 search results, got %d", len(hits))
	}
	if hits[0].Slug == hits[1].Slug {
		t.Fatalf("expected distinct slugs for two documents with the same title and author, got %q for both", hits[0].Slug)
	}
}
