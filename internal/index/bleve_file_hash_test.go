package index

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
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

func newHashTestIndex(t *testing.T, fs afero.Fs, readers map[string]metadata.Reader) *BleveIndexer {
	t.Helper()
	documents, err := bleve.NewMemOnly(CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}
	authors, err := bleve.NewMemOnly(CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}
	idx := NewBleve(documents, authors, fs, "lib", readers, Config{})
	t.Cleanup(func() { _ = idx.Close() })
	return idx
}

func (hashTestReader) Metadata(path string) (metadata.Metadata, error) {
	return metadata.Metadata{Title: "Book", Format: strings.ToUpper(strings.TrimPrefix(filepath.Ext(path), "."))}, nil
}

func (hashTestReader) Cover(string, int) (image.Image, error) {
	return nil, nil
}

type blockingLibraryReader struct {
	hashTestReader
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *blockingLibraryReader) Metadata(path string) (metadata.Metadata, error) {
	r.once.Do(func() {
		close(r.started)
		<-r.release
	})
	meta, err := r.hashTestReader.Metadata(path)
	meta.Authors = []string{"Writer"}
	return meta, err
}

func TestStartupIndexingRejectsMutations(t *testing.T) {
	fs := afero.NewMemMapFs()
	if err := afero.WriteFile(fs, "lib/book.epub", []byte("original content"), 0644); err != nil {
		t.Fatal(err)
	}
	reader := &blockingLibraryReader{started: make(chan struct{}), release: make(chan struct{})}
	var release sync.Once
	unblock := func() { release.Do(func() { close(reader.release) }) }
	t.Cleanup(unblock)
	idx := newHashTestIndex(t, fs, map[string]metadata.Reader{".epub": reader})
	idx.BeginIndexing()
	checkRejected := func() {
		t.Helper()
		if _, err := idx.NewFile("rejected.epub", []byte("upload")); err != ErrLibraryIndexing {
			t.Fatalf("upload during indexing returned %v", err)
		}
		if err := idx.DeleteDocument("writer-book"); err != ErrLibraryIndexing {
			t.Fatalf("delete during indexing returned %v", err)
		}
		if exists, err := afero.Exists(fs, "lib/rejected.epub"); err != nil || exists {
			t.Fatalf("rejected upload wrote a file: exists=%v, err=%v", exists, err)
		}
	}
	checkRejected()
	bulk := make(chan error, 1)
	go func() { bulk <- idx.AddLibrary(1, true, 1) }()
	<-reader.started
	checkRejected()
	if _, err := idx.TotalDocs(); err != nil {
		t.Fatalf("reads unavailable during indexing: %v", err)
	}
	data, err := afero.ReadFile(fs, "lib/book.epub")
	if err != nil || string(data) != "original content" {
		t.Fatalf("file changed during bulk indexing: %q, %v", data, err)
	}
	unblock()
	if err := <-bulk; err != nil {
		t.Fatal(err)
	}
	if _, err := idx.NewFile("book.epub", []byte("modified content")); err != nil {
		t.Fatalf("retry after indexing failed: %v", err)
	}
	count, err := idx.TotalDocs()
	if err != nil || count != 1 {
		t.Fatalf("document count = %d, %v; want 1", count, err)
	}
	author, err := idx.Author("writer", "")
	if err != nil || author.DocumentCount != 1 {
		t.Fatalf("author count = %d, %v; want 1", author.DocumentCount, err)
	}
	if err := idx.DeleteDocument("writer-book"); err != nil {
		t.Fatalf("delete after indexing failed: %v", err)
	}
	count, err = idx.TotalDocs()
	if err != nil || count != 0 {
		t.Fatalf("document count after deletion = %d, %v; want 0", count, err)
	}
	author, err = idx.Author("writer", "")
	if err != nil || author.DocumentCount != 0 {
		t.Fatalf("author count after deletion = %d, %v; want 0", author.DocumentCount, err)
	}
}

func TestIndexFileUpdatesHashWithoutMetadataChanges(t *testing.T) {
	fs := &hashCountingFs{Fs: afero.NewMemMapFs()}
	idx := newHashTestIndex(t, fs, map[string]metadata.Reader{".epub": hashTestReader{}})
	slug, err := idx.NewFile("book.epub", []byte("original content"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := idx.Document(slug)
	if err != nil {
		t.Fatal(err)
	}
	doc.TextRankWords = []string{"enriched"}
	if err := idx.documentsIdx.Index(doc.ID, doc); err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{"original content", "modified content", "modified content"} {
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
		if content == "modified content" {
			if !reflect.DeepEqual(state.(indexedFileState).document.TextRankWords, doc.TextRankWords) {
				t.Fatal("in-memory document state does not reflect the reindexed document")
			}
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
			{name: "matching ETag omits bytes", conditional: true},
			{name: "unconditional read returns bytes"},
			{name: "same size change", content: "modified content", changeTime: true, conditional: true},
			{name: "same size and timestamp change", content: "modified content", conditional: true},
			{name: "size change with unchanged timestamp", content: "longer modified content", conditional: true},
			{name: "missing file rejects cached hash", missingFile: true, conditional: true},
		} {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
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
				idx := newHashTestIndex(t, fs, map[string]metadata.Reader{"." + format: hashTestReader{}})
				if err := idx.AddLibrary(1, true, 1); err != nil {
					t.Fatal(err)
				}
				if fs.opens != 0 {
					t.Fatal("bulk indexing read file contents beyond metadata extraction")
				}

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
