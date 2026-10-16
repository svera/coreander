package index

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/pirmd/epub"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/metadata"
)

func newAuthorMetadataIndex(t *testing.T) (*BleveIndexer, string) {
	t.Helper()
	documents, err := bleve.NewMemOnly(CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}
	authors, err := bleve.NewMemOnly(CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}
	library := t.TempDir()
	idx := NewBleve(documents, authors, afero.NewOsFs(), library,
		map[string]metadata.Reader{".epub": metadata.NewEpubReader()}, Config{})
	t.Cleanup(func() { _ = idx.Close() })
	return idx, library
}

func authorMetadataEPUB(t *testing.T) []byte {
	t.Helper()
	var data bytes.Buffer
	w := zip.NewWriter(&data)
	for name, content := range map[string]string{
		"META-INF/container.xml": `<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container" version="1.0"><rootfiles><rootfile full-path="book.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`,
		"book.opf": `<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="id">
<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
<dc:identifier id="id">book</dc:identifier><dc:title>Book</dc:title><dc:language>en</dc:language>
<dc:creator id="author">Douglas Adams</dc:creator></metadata><manifest/><spine/></package>`,
	} {
		entry, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func requireWrittenAuthorID(t *testing.T, filename, want string) {
	t.Helper()
	info, err := epub.GetMetadataFromFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, m := range info.Creator[0].Refinements {
		if m.Property == metadata.WikidataAuthorIDsProperty {
			ids = append(ids, m.Value)
		}
	}
	if (want == "" && len(ids) != 0) || (want != "" && (len(ids) != 1 || ids[0] != want)) {
		t.Fatalf("written IDs %v, want %q", ids, want)
	}
}

func TestIndexAuthorWritesWikidataIDs(t *testing.T) {
	idx, library := newAuthorMetadataIndex(t)
	documentSlug, err := idx.NewFile("book.epub", authorMetadataEPUB(t))
	if err != nil {
		t.Fatal(err)
	}
	before, err := idx.Document(documentSlug)
	if err != nil {
		t.Fatal(err)
	}
	author, err := idx.Author("douglas-adams", "")
	if err != nil {
		t.Fatal(err)
	}
	author.DataSourceID = "Q42"
	if err := idx.IndexAuthor(author); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(library, "book.epub")
	requireWrittenAuthorID(t, filename, "Q42")
	state, ok := idx.lastIndexed.Load("book.epub")
	if !ok {
		t.Fatal("missing cached document")
	}
	hash, err := idx.fileHash(filename)
	if err != nil || state.(indexedFileState).hash != hash {
		t.Fatalf("cached hash does not match updated EPUB: %v", err)
	}
	if _, err := idx.indexFile(filename); err != nil {
		t.Fatal(err)
	}
	after, err := idx.Document(documentSlug)
	if err != nil {
		t.Fatal(err)
	}
	author, err = idx.Author("douglas-adams", "")
	if err != nil {
		t.Fatal(err)
	}
	if !after.AddedOn.Equal(before.AddedOn) || author.DocumentCount != 1 {
		t.Fatal("metadata write caused a duplicate indexing event")
	}

	if _, err := idx.NewFile("new.epub", authorMetadataEPUB(t)); err != nil {
		t.Fatal(err)
	}
	requireWrittenAuthorID(t, filepath.Join(library, "new.epub"), "Q42")
	author.DataSourceID = "Q99"
	if err := idx.IndexAuthor(author); err != nil {
		t.Fatal(err)
	}
	requireWrittenAuthorID(t, filename, "Q99")
	requireWrittenAuthorID(t, filepath.Join(library, "new.epub"), "Q99")
	author.DataSourceID = ""
	if err := idx.IndexAuthor(author); err != nil {
		t.Fatal(err)
	}
	requireWrittenAuthorID(t, filename, "")
}

func TestIndexAuthorWritesEveryDocumentPage(t *testing.T) {
	idx, library := newAuthorMetadataIndex(t)
	data := authorMetadataEPUB(t)
	for i := range 101 {
		if err := os.WriteFile(filepath.Join(library, fmt.Sprintf("%03d.epub", i)), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := idx.AddLibrary(25, false, 1); err != nil {
		t.Fatal(err)
	}
	if err := idx.RebuildAuthorsFromDocuments(25); err != nil {
		t.Fatal(err)
	}
	author, err := idx.Author("douglas-adams", "")
	if err != nil {
		t.Fatal(err)
	}
	author.DataSourceID = "Q42"
	if err := idx.IndexAuthor(author); err != nil {
		t.Fatal(err)
	}
	for i := range 101 {
		requireWrittenAuthorID(t, filepath.Join(library, fmt.Sprintf("%03d.epub", i)), "Q42")
	}

	filename := filepath.Join(library, "later.epub")
	if err := os.WriteFile(filename, data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := idx.AddLibrary(25, false, 1); err != nil {
		t.Fatal(err)
	}
	requireWrittenAuthorID(t, filename, "Q42")
}

func TestIndexAuthorSurfacesMetadataWriteFailure(t *testing.T) {
	idx, library := newAuthorMetadataIndex(t)
	if _, err := idx.NewFile("book.epub", authorMetadataEPUB(t)); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(library, "book.epub"), []byte("invalid EPUB"), 0644); err != nil {
		t.Fatal(err)
	}
	err := idx.IndexAuthor(Author{Name: "Douglas Adams", Slug: "douglas-adams", DataSourceID: "Q42"})
	if err == nil || !strings.Contains(err.Error(), "writing author Wikidata IDs") {
		t.Fatalf("expected metadata write failure, got %v", err)
	}
	author, err := idx.Author("douglas-adams", "")
	if err != nil {
		t.Fatal(err)
	}
	if author.DataSourceID != "" {
		t.Fatal("failed metadata write was marked as successfully indexed")
	}
}

func TestIndexAuthorConcurrentFileIndexing(t *testing.T) {
	idx, library := newAuthorMetadataIndex(t)
	if _, err := idx.NewFile("book.epub", authorMetadataEPUB(t)); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Go(func() {
			if err := idx.IndexAuthor(Author{
				Name: "Douglas Adams", Slug: "douglas-adams",
				DataSourceID: fmt.Sprintf("Q%d", i+1), DocumentCount: 1,
			}); err != nil {
				t.Error(err)
			}
		})
		wg.Go(func() {
			if _, err := idx.indexFile(filepath.Join(library, "book.epub")); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	author, err := idx.Author("douglas-adams", "")
	if err != nil {
		t.Fatal(err)
	}
	requireWrittenAuthorID(t, filepath.Join(library, "book.epub"), author.DataSourceID)
}
