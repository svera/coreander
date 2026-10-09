package index

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/metadata"
)

type hashCountingFs struct {
	afero.Fs
	opens int
}

func (fs *hashCountingFs) Open(name string) (afero.File, error) {
	if info, err := fs.Fs.Stat(name); err == nil && !info.IsDir() {
		fs.opens++
	}
	return fs.Fs.Open(name)
}

type hashTestReader struct{}

func (hashTestReader) Metadata(path string) (metadata.Metadata, error) {
	return metadata.Metadata{Title: "Book", Format: strings.ToUpper(strings.TrimPrefix(filepath.Ext(path), "."))}, nil
}

func (hashTestReader) Cover(string, int) (image.Image, error) {
	return nil, nil
}

func TestIndexFileUpdatesHashWithoutMetadataChanges(t *testing.T) {
	documents, err := bleve.NewMemOnly(CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}
	authors, err := bleve.NewMemOnly(CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}
	fs := &hashCountingFs{Fs: afero.NewMemMapFs()}
	idx := NewBleve(documents, authors, fs, "lib",
		map[string]metadata.Reader{".epub": hashTestReader{}}, Config{})
	t.Cleanup(func() { _ = idx.Close() })
	slug, err := idx.NewFile("book.epub", []byte("original content"))
	if err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{"original content", "modified content"} {
		if err := afero.WriteFile(fs, "lib/book.epub", []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		now := time.Now().Add(time.Second)
		if err := fs.Chtimes("lib/book.epub", now, now); err != nil {
			t.Fatal(err)
		}
		gotSlug, err := idx.indexFile("lib/book.epub")
		if err != nil {
			t.Fatal(err)
		}
		if gotSlug != slug {
			t.Fatalf("slug changed from %q to %q", slug, gotSlug)
		}
		fs.opens = 0
		file, err := idx.File(slug, fmt.Sprintf(`"%x"`, sha256.Sum256([]byte(content))))
		if err != nil {
			t.Fatal(err)
		}
		if file.Data != nil || fs.opens != 1 {
			t.Fatal("conditional download did not read and validate the current content")
		}
		state, ok := idx.lastIndexed.Load("book.epub")
		if !ok || state.(indexedFileState).hash != fmt.Sprintf("%x", sha256.Sum256([]byte(content))) {
			t.Fatal("indexing did not update the in-memory content hash")
		}
	}
}

func TestFileHash(t *testing.T) {
	for _, format := range []string{"epub", "pdf"} {
		for _, tc := range []struct {
			name        string
			content     string
			changeTime  bool
			conditional bool
			missingFile bool
		}{
			{name: "conditional read after enrichment and restart", conditional: true},
			{name: "unconditional read returns bytes"},
			{name: "same size change", content: "modified content", changeTime: true, conditional: true},
			{name: "same size and timestamp change", content: "modified content", conditional: true},
			{name: "size change with unchanged timestamp", content: "longer modified content", conditional: true},
			{name: "missing file rejects cached hash", missingFile: true, conditional: true},
		} {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
				indexPath := filepath.Join(t.TempDir(), "documents")
				documents, err := bleve.New(indexPath, CreateDocumentsMapping())
				if err != nil {
					t.Fatal(err)
				}
				authors, err := bleve.NewMemOnly(CreateAuthorsMapping())
				if err != nil {
					t.Fatal(err)
				}
				fs := &hashCountingFs{Fs: afero.NewMemMapFs()}
				path := "lib/book." + format
				data := []byte("original content")
				modTime := time.Unix(1700000000, 123456789)
				writeFile := func() {
					t.Helper()
					if err := afero.WriteFile(fs, path, data, 0644); err != nil {
						t.Fatal(err)
					}
					if err := fs.Chtimes(path, modTime, modTime); err != nil {
						t.Fatal(err)
					}
				}

				writeFile()
				originalETag := fmt.Sprintf(`"%x"`, sha256.Sum256(data))
				idx := NewBleve(documents, authors, fs, "lib",
					map[string]metadata.Reader{"." + format: hashTestReader{}}, Config{})
				t.Cleanup(func() { _ = idx.Close() })
				if err := idx.AddLibrary(1, true, 1); err != nil {
					t.Fatal(err)
				}
				if fs.opens != 0 {
					t.Fatal("bulk indexing read file contents beyond metadata extraction")
				}
				doc, err := idx.Document("book")
				if err != nil {
					t.Fatal(err)
				}
				idx.enrichTextRankAndReindex(doc)
				if err := idx.Close(); err != nil {
					t.Fatal(err)
				}
				documents, err = bleve.Open(indexPath)
				if err != nil {
					t.Fatal(err)
				}
				authors, err = bleve.NewMemOnly(CreateAuthorsMapping())
				if err != nil {
					t.Fatal(err)
				}
				idx = NewBleve(documents, authors, fs, "lib", nil, Config{})

				if tc.content != "" {
					data = []byte(tc.content)
					if tc.changeTime {
						modTime = modTime.Add(time.Second)
					}
					writeFile()
				}
				if tc.missingFile {
					if err := fs.Remove(path); err != nil {
						t.Fatal(err)
					}
				}
				before, err := idx.Document("book")
				if err != nil {
					t.Fatal(err)
				}
				fs.opens = 0
				conditional := ""
				if tc.conditional {
					conditional = originalETag
				}
				file, err := idx.File("book", conditional)
				if tc.missingFile {
					if err != ErrDocumentNotFound {
						t.Fatalf("missing file error = %v", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if want := fmt.Sprintf(`"%x"`, sha256.Sum256(data)); file.ETag != want {
					t.Errorf("ETag = %q, want %q", file.ETag, want)
				}
				unchanged := tc.conditional && tc.content == ""
				if unchanged {
					if file.Data != nil || fs.opens != 1 {
						t.Error("unchanged validation did not read the file and omit the response body")
					}
				} else if !bytes.Equal(file.Data, data) || fs.opens != 1 {
					t.Error("download did not read and return the complete file")
				}
				if tc.content != "" {
					validated, err := idx.File("book", file.ETag)
					if err != nil {
						t.Fatal(err)
					}
					if validated.ETag != file.ETag || validated.Data != nil {
						t.Error("current content ETag did not validate without a response body")
					}
				}
				after, err := idx.Document("book")
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(before, after) {
					t.Error("download modified the indexed document")
				}
			})
		}
	}
}
