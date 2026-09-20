package index_test

import (
	"path/filepath"
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/metadata"
)

// TestNewFileRejectsPathTraversalInFileName guards against a client-supplied
// upload filename escaping the library directory. NewFile is reachable from
// the document upload endpoint with an attacker-controlled multipart
// filename, so "../../etc/cron.d/evil.epub" must not be written outside
// libraryPath.
func TestNewFileRejectsPathTraversalInFileName(t *testing.T) {
	fs := afero.NewMemMapFs()
	const lib = "var/lib"
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

	slug, err := idx.NewFile("../../etc/cron.d/evil.epub", []byte("x"))
	if err != nil {
		t.Fatalf("NewFile returned an error: %v", err)
	}

	doc, err := idx.Document(slug)
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	if doc.ID != "evil.epub" {
		t.Fatalf("expected the traversal segments to be stripped from the file name, got ID %q", doc.ID)
	}

	if exists, err := afero.Exists(fs, filepath.Join(lib, "evil.epub")); err != nil {
		t.Fatalf("checking for the indexed file: %v", err)
	} else if !exists {
		t.Fatalf("expected the file to be written inside %q", lib)
	}

	if exists, err := afero.Exists(fs, "etc/cron.d/evil.epub"); err != nil {
		t.Fatalf("checking for escaped file: %v", err)
	} else if exists {
		t.Fatalf("path traversal in the upload filename wrote a file outside %q", lib)
	}
}
