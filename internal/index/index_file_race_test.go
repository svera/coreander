package index_test

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/metadata"
)

// TestNewFileConcurrentUploadsOfSameFileAreIdempotent simulates upload and
// the file watcher racing to index the same file (both funnel into indexFile
// via NewFile / the watcher's own call), asserting the race can't produce a
// colliding slug with a pre-existing document sharing title/author, or
// double-count author stats.
func TestNewFileConcurrentUploadsOfSameFileAreIdempotent(t *testing.T) {
	fs := afero.NewMemMapFs()
	const lib = "lib"
	if err := fs.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
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

	existingSlug, err := idx.NewFile("ivanhoe1.epub", []byte("x"))
	if err != nil {
		t.Fatalf("indexing pre-existing document: %v", err)
	}

	const races = 8
	var wg sync.WaitGroup
	slugs := make([]string, races)
	for i := 0; i < races; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			slug, err := idx.NewFile("ivanhoe2.epub", []byte("x"))
			if err != nil {
				t.Errorf("NewFile race %d: %v", i, err)
				return
			}
			slugs[i] = slug
		}(i)
	}
	wg.Wait()

	for _, slug := range slugs {
		if slug != slugs[0] {
			t.Fatalf("expected all concurrent uploads of the same file to agree on one slug, got %q and %q", slugs[0], slug)
		}
	}
	racingSlug := slugs[0]

	if racingSlug == existingSlug {
		t.Fatalf("expected distinct slugs for two documents with the same title/author, got %q for both", racingSlug)
	}

	total, err := idx.TotalDocs()
	if err != nil {
		t.Fatalf("TotalDocs: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected 2 indexed documents (one per file), got %d", total)
	}

	racingDoc, err := idx.Document(racingSlug)
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	if filepath.Base(racingDoc.ID) != "ivanhoe2.epub" {
		t.Fatalf("expected the racing slug to resolve to ivanhoe2.epub, got %q", racingDoc.ID)
	}

	author, err := idx.Author(racingDoc.AuthorsSlugs[0], "")
	if err != nil {
		t.Fatalf("Author: %v", err)
	}
	if author.DocumentCount != 2 {
		t.Fatalf("expected author document count 2 (not double-counted by the race), got %d", author.DocumentCount)
	}
}
