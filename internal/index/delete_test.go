package index_test

import (
	"path/filepath"
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/index"
)

func newDeleteTestIndex(t *testing.T, fs afero.Fs, lib string) *index.BleveIndexer {
	t.Helper()
	docIdx, err := bleve.NewMemOnly(index.CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}
	authIdx, err := bleve.NewMemOnly(index.CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}
	return index.NewBleve(docIdx, authIdx, fs, lib, benchmarkReaders(), index.Config{})
}

func slugForDeleteTest(t *testing.T, idx *index.BleveIndexer, keywords string) string {
	t.Helper()
	results, err := idx.Search(index.SearchFields{Keywords: keywords}, 1, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	hits := results.Hits()
	if len(hits) != 1 {
		t.Fatalf("expected 1 search result for %q, got %d", keywords, len(hits))
	}
	return hits[0].Slug
}

// TestDeleteDocument covers deleting a document whose bleve ID may be a full path relative to
// the library root (e.g. "nested/book.epub"), including the case where the underlying file was
// already removed from disk (e.g. manually, outside the app) before deletion is requested:
// DeleteDocument must still succeed in removing the entry from the index in both cases.
func TestDeleteDocument(t *testing.T) {
	tests := map[string]struct {
		relPath            string
		removeBeforeDelete bool
	}{
		"flat file": {
			relPath: "book.epub",
		},
		"nested file": {
			relPath: filepath.Join("nested", "book.epub"),
		},
		"missing file": {
			relPath:            "book.epub",
			removeBeforeDelete: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			const lib = "lib"
			if err := fs.MkdirAll(filepath.Join(lib, filepath.Dir(tc.relPath)), 0o755); err != nil {
				t.Fatal(err)
			}
			fullPath := filepath.Join(lib, tc.relPath)
			if err := afero.WriteFile(fs, fullPath, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}

			idx := newDeleteTestIndex(t, fs, lib)
			defer idx.Close()

			if err := idx.AddLibrary(10, true, 1); err != nil {
				t.Fatalf("AddLibrary: %v", err)
			}

			slug := slugForDeleteTest(t, idx, "book.epub")

			if tc.removeBeforeDelete {
				if err := fs.Remove(fullPath); err != nil {
					t.Fatalf("failed to remove file to simulate missing file: %v", err)
				}
			}

			if err := idx.DeleteDocument(slug); err != nil {
				t.Fatalf("DeleteDocument returned error: %v", err)
			}

			n, err := idx.TotalDocs()
			if err != nil {
				t.Fatal(err)
			}
			if n != 0 {
				t.Errorf("expected document to be removed from index, but %d documents remain", n)
			}
			if exists, _ := afero.Exists(fs, fullPath); exists {
				t.Errorf("expected file %s to be removed from filesystem, but it still exists", fullPath)
			}
		})
	}
}
