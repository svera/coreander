package index_test

import (
	"sync"
	"testing"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/metadata"
)

type slowMetadataReader struct {
	delay time.Duration
}

func (r slowMetadataReader) Metadata(string) (metadata.Metadata, error) {
	time.Sleep(r.delay)
	return metadata.Metadata{Title: "t", Authors: []string{"a"}, Format: "EPUB"}, nil
}

func (slowMetadataReader) Cover(string, int) ([]byte, error) {
	return nil, nil
}

func TestIndexingProgressDuringMetadata(t *testing.T) {
	fs := afero.NewMemMapFs()
	const lib = "lib"
	if err := fs.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, lib+"/a.epub", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, lib+"/b.epub", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	docIdx, _ := bleve.NewMemOnly(index.CreateDocumentsMapping())
	authIdx, _ := bleve.NewMemOnly(index.CreateAuthorsMapping())
	readers := map[string]metadata.Reader{".epub": slowMetadataReader{delay: 30 * time.Millisecond}}
	idx := index.NewBleve(docIdx, authIdx, fs, lib, readers, index.Config{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = idx.AddLibrary(10, true, 1)
	}()

	deadline := time.Now().Add(2 * time.Second)
	sawProgress := false
	for time.Now().Before(deadline) {
		p, err := idx.IndexingProgress()
		if err != nil {
			t.Fatal(err)
		}
		if p.InProgress && p.Percentage > 0 {
			sawProgress = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	wg.Wait()
	_ = idx.Close()

	if !sawProgress {
		t.Fatal("expected IndexingProgress to report percentage > 0 while metadata was running")
	}
}
