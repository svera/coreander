package index

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/metadata"
)

type hashCountingFs struct {
	afero.Fs
	opens atomic.Int64
}

func (fs *hashCountingFs) Open(name string) (afero.File, error) {
	fs.opens.Add(1)
	return fs.Fs.Open(name)
}

func TestFileHash(t *testing.T) {
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
	const path = "lib/book.epub"
	data := []byte("original content")
	if err := afero.WriteFile(fs, path, data, 0644); err != nil {
		t.Fatal(err)
	}
	modTime := time.Unix(1700000000, 123456789)
	if err := fs.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}
	doc := Document{
		ID: "book.epub", Slug: "book",
		Metadata:         metadata.Metadata{Title: "Book", Format: "EPUB"},
		TextRankEnriched: true,
		TextRankWords:    []string{"original"},
	}
	if err := documents.Index(doc.ID, doc); err != nil {
		t.Fatal(err)
	}
	idx := NewBleve(documents, authors, fs, "lib", nil, Config{})
	t.Cleanup(func() { _ = idx.Close() })
	expected := fmt.Sprintf(`"%x"`, sha256.Sum256(data))

	readFile := func(t *testing.T, conditional string, wantData []byte) *IndexedFile {
		t.Helper()
		file, err := idx.File("book", conditional)
		if err != nil {
			t.Fatal(err)
		}
		if file.ETag != expected || !bytes.Equal(file.Data, wantData) || (wantData == nil && file.Data != nil) {
			t.Fatalf("ETag = %q, payload = %q; want %q, %q", file.ETag, file.Data, expected, wantData)
		}
		return file
	}
	validateConcurrently := func(t *testing.T, backgroundWrites bool) {
		t.Helper()
		var wg sync.WaitGroup
		for range 8 {
			if backgroundWrites {
				wg.Go(func() {
					if err := idx.persistTextRankDocuments([]Document{doc}, true); err != nil {
						t.Error(err)
					}
				})
			}
			wg.Go(func() {
				file, err := idx.File("book", expected)
				if err != nil {
					t.Error(err)
					return
				}
				if file.Data != nil || file.ETag != expected {
					t.Error("concurrent validation did not reuse hash")
				}
			})
		}
		wg.Wait()
	}

	t.Run("backfills existing index on first download", func(t *testing.T) {
		readFile(t, "", data)
		if fs.opens.Load() != 1 {
			t.Fatalf("file opens = %d, want 1", fs.opens.Load())
		}
		stored, err := idx.Document("book")
		if err != nil {
			t.Fatal(err)
		}
		info, err := fs.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if `"`+stored.ContentHash+`"` != expected || stored.ContentSize != int64(len(data)) || stored.ContentModTime != info.ModTime().UTC().Format(time.RFC3339Nano) {
			t.Fatalf("stored hash fields = %#v", stored)
		}
	})

	t.Run("unchanged conditional requests do not open file", func(t *testing.T) {
		for range 3 {
			readFile(t, expected, nil)
		}
		if fs.opens.Load() != 1 {
			t.Fatalf("file opens = %d, want 1", fs.opens.Load())
		}
	})

	t.Run("survives document enrichment and index reopen", func(t *testing.T) {
		doc.TextRankWords = []string{"enriched"}
		if err := idx.persistTextRankDocuments([]Document{doc}, true); err != nil {
			t.Fatal(err)
		}
		if err := idx.persistTextRankDocuments([]Document{doc}, false); err != nil {
			t.Fatal(err)
		}
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
		file := readFile(t, expected, nil)
		if fs.opens.Load() != 1 {
			t.Fatal("persisted hash was not reused")
		}
		if len(file.Document.TextRankWords) != 1 || file.Document.TextRankWords[0] != "enriched" {
			t.Fatal("enrichment metadata was lost")
		}
	})

	t.Run("same size content change refreshes hash", func(t *testing.T) {
		info, err := fs.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		data = []byte("modified content")
		if err := afero.WriteFile(fs, path, data, 0644); err != nil {
			t.Fatal(err)
		}
		changed := info.ModTime().Add(time.Second)
		if err := fs.Chtimes(path, changed, changed); err != nil {
			t.Fatal(err)
		}
		oldETag := expected
		expected = fmt.Sprintf(`"%x"`, sha256.Sum256(data))
		readFile(t, oldETag, data)
		if fs.opens.Load() != 2 {
			t.Fatal("changed file did not refresh hash and payload")
		}
	})

	t.Run("concurrent validation reuses cached hash", func(t *testing.T) {
		validateConcurrently(t, false)
		if fs.opens.Load() != 2 {
			t.Fatalf("file opens = %d, want 2", fs.opens.Load())
		}
	})

	t.Run("size change invalidates hash even with unchanged modification time", func(t *testing.T) {
		info, err := fs.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		data = append(data, []byte(" extended")...)
		if err := afero.WriteFile(fs, path, data, 0644); err != nil {
			t.Fatal(err)
		}
		if err := fs.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
			t.Fatal(err)
		}
		oldETag := expected
		expected = fmt.Sprintf(`"%x"`, sha256.Sum256(data))
		readFile(t, oldETag, data)
		if fs.opens.Load() != 3 {
			t.Fatal("size change did not refresh hash and payload")
		}
	})

	t.Run("missing file is not served from hash cache", func(t *testing.T) {
		if err := fs.Rename(path, path+".bak"); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := fs.Rename(path+".bak", path); err != nil {
				t.Error(err)
			}
		}()
		if _, err := idx.File("book", expected); err != ErrDocumentNotFound {
			t.Fatalf("missing file error = %v", err)
		}
	})

	t.Run("concurrent backfill reads the file only once", func(t *testing.T) {
		current, err := idx.Document("book")
		if err != nil {
			t.Fatal(err)
		}
		current.ContentHash = ""
		current.ContentSize = 0
		current.ContentModTime = ""
		if err := documents.Index(current.ID, current); err != nil {
			t.Fatal(err)
		}
		validateConcurrently(t, false)
		if fs.opens.Load() != 4 {
			t.Fatalf("file opens = %d, want 4", fs.opens.Load())
		}
	})

	t.Run("unconditional reads return bytes even with a cached hash", func(t *testing.T) {
		readFile(t, "", data)
	})

	t.Run("concurrent background writes preserve hash fields", func(t *testing.T) {
		validateConcurrently(t, true)
	})

	t.Run("deletion removes persisted hash", func(t *testing.T) {
		if err := idx.DeleteDocument("book"); err != nil {
			t.Fatal(err)
		}
		stored, err := idx.documentByIndexID(doc.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stored.ContentHash != "" || stored.ID != "" {
			t.Fatal("deleted document hash remains in index")
		}
		if err := idx.persistTextRankDocuments([]Document{doc}, true); err != nil {
			t.Fatal(err)
		}
		if _, err := idx.File("book", expected); err != ErrDocumentNotFound {
			t.Fatalf("deleted document error = %v", err)
		}
	})
}
