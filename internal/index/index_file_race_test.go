package index

import (
	"html/template"
	"image"
	"path/filepath"
	"sync"
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/metadata"
)

// raceTestReader always reports the same title and author regardless of the
// file being read, simulating two distinct files in the library that happen
// to share title and author.
type raceTestReader struct{}

func (raceTestReader) Metadata(file string) (metadata.Metadata, error) {
	return metadata.Metadata{
		Title:       "Ivanhoe",
		Authors:     []string{"Walter Scott"},
		Description: template.HTML("<p>same title and author</p>"),
		Language:    "en",
		Format:      "EPUB",
		Words:       1000,
	}, nil
}

func (raceTestReader) Cover(string, int) (image.Image, error) {
	return nil, nil
}

// TestIndexFileConcurrentCallsForSameFileAreIdempotent simulates the race
// between an upload (NewFile calling indexFile directly) and the Linux file
// watcher reacting to that same write with its own indexFile call for the
// same path. Without serialization, both calls independently pick a slug
// against the live index and can end up colliding with a pre-existing
// document sharing the same title/author, or double-count author stats.
func TestIndexFileConcurrentCallsForSameFileAreIdempotent(t *testing.T) {
	fs := afero.NewMemMapFs()
	const lib = "lib"
	if err := fs.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}

	existingPath := filepath.Join(lib, "ivanhoe1.epub")
	racingPath := filepath.Join(lib, "ivanhoe2.epub")
	for _, path := range []string{existingPath, racingPath} {
		if err := afero.WriteFile(fs, path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	docIdx, err := bleve.NewMemOnly(CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}
	authIdx, err := bleve.NewMemOnly(CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}
	readers := map[string]metadata.Reader{".epub": raceTestReader{}}
	idx := NewBleve(docIdx, authIdx, fs, lib, readers, Config{})
	defer idx.Close()

	if _, err := idx.indexFile(existingPath); err != nil {
		t.Fatalf("indexing pre-existing document: %v", err)
	}

	const races = 8
	var wg sync.WaitGroup
	slugs := make([]string, races)
	for i := 0; i < races; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			slug, err := idx.indexFile(racingPath)
			if err != nil {
				t.Errorf("indexFile race %d: %v", i, err)
				return
			}
			slugs[i] = slug
		}(i)
	}
	wg.Wait()

	for _, slug := range slugs {
		if slug != slugs[0] {
			t.Fatalf("expected all concurrent indexFile calls for the same path to agree on one slug, got %q and %q", slugs[0], slug)
		}
	}

	total, err := idx.TotalDocs()
	if err != nil {
		t.Fatalf("TotalDocs: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected 2 indexed documents (one per file), got %d", total)
	}

	racingDoc, err := idx.documentByIndexID(idx.id(racingPath))
	if err != nil {
		t.Fatalf("documentByIndexID: %v", err)
	}
	if racingDoc.Slug != slugs[0] {
		t.Fatalf("expected racing document's stored slug %q to match returned slug %q", racingDoc.Slug, slugs[0])
	}

	existingDoc, err := idx.documentByIndexID(idx.id(existingPath))
	if err != nil {
		t.Fatalf("documentByIndexID for pre-existing doc: %v", err)
	}
	if existingDoc.Slug == racingDoc.Slug {
		t.Fatalf("expected distinct slugs for two documents with the same title/author, got %q for both", existingDoc.Slug)
	}

	author, err := idx.Author(existingDoc.AuthorsSlugs[0], "")
	if err != nil {
		t.Fatalf("Author: %v", err)
	}
	if author.DocumentCount != 2 {
		t.Fatalf("expected author document count 2 (not double-counted by the race), got %d", author.DocumentCount)
	}
}
