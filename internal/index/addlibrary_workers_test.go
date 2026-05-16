package index_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/metadata"
)

func TestAddLibraryParallelMetadataWorkers(t *testing.T) {
	fs := afero.NewMemMapFs()
	const lib = "lib"
	if err := fs.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := range 5 {
		name := filepath.Join(lib, fmt.Sprintf("book-%d.epub", i))
		if err := afero.WriteFile(fs, name, []byte("x"), 0o644); err != nil {
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
	readers := map[string]metadata.Reader{".epub": benchmarkEpubReader{}}
	idx := index.NewBleve(docIdx, authIdx, fs, lib, readers, index.Config{})

	if err := idx.AddLibrary(10, true, 4); err != nil {
		t.Fatalf("AddLibrary: %v", err)
	}
	n, err := idx.Count()
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("expected 5 indexed documents, got %d", n)
	}
	_ = idx.Close()
}
