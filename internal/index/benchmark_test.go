package index_test

import (
	"fmt"
	"html/template"
	"path/filepath"
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/metadata"
)

// benchmarkEpubReader returns cheap metadata so benchmarks measure indexer and
// Bleve work, not real EPUB parsing.
type benchmarkEpubReader struct{}

func (benchmarkEpubReader) Metadata(file string) (metadata.Metadata, error) {
	base := filepath.Base(file)
	return metadata.Metadata{
		Title:       base,
		Authors:     []string{"Benchmark Author"},
		Description: template.HTML("<p>bench</p>"),
		Language:    "en",
		Format:      "EPUB",
		Words:       1000,
		Subjects:    []string{"Fiction"},
	}, nil
}

func (benchmarkEpubReader) Cover(string, int) ([]byte, error) {
	return nil, nil
}

func benchmarkReaders() map[string]metadata.Reader {
	return map[string]metadata.Reader{
		".epub": benchmarkEpubReader{},
	}
}

func newMemBleveIndexer(b *testing.B, fs afero.Fs, libPath string) *index.BleveIndexer {
	b.Helper()
	docIdx, err := bleve.NewMemOnly(index.CreateDocumentsMapping())
	if err != nil {
		b.Fatalf("documents index: %v", err)
	}
	authIdx, err := bleve.NewMemOnly(index.CreateAuthorsMapping())
	if err != nil {
		b.Fatalf("authors index: %v", err)
	}
	return index.NewBleve(docIdx, authIdx, fs, libPath, benchmarkReaders(), index.Config{
		IllustratedMinAmount: 2,
		IllustratedMinSize:   0.25,
	})
}

func populateEPUBs(b *testing.B, fs afero.Fs, libPath string, n int) {
	b.Helper()
	if err := fs.MkdirAll(libPath, 0o755); err != nil {
		b.Fatalf("mkdir: %v", err)
	}
	for i := range n {
		name := filepath.Join(libPath, fmt.Sprintf("book-%06d.epub", i))
		if err := afero.WriteFile(fs, name, []byte("stub"), 0o644); err != nil {
			b.Fatalf("write %s: %v", name, err)
		}
	}
}

func populateNoiseFiles(b *testing.B, fs afero.Fs, libPath string, n int) {
	b.Helper()
	if err := fs.MkdirAll(libPath, 0o755); err != nil {
		b.Fatalf("mkdir: %v", err)
	}
	for i := range n {
		name := filepath.Join(libPath, fmt.Sprintf("noise-%06d.txt", i))
		if err := afero.WriteFile(fs, name, []byte("noise"), 0o644); err != nil {
			b.Fatalf("write %s: %v", name, err)
		}
	}
}

// BenchmarkAddLibrary_ForceIndex measures a cold index build with fast metadata.
func BenchmarkAddLibrary_ForceIndex(b *testing.B) {
	sizes := []int{10, 50, 200}
	for _, n := range sizes {
		b.Run(fmt.Sprintf("epubs_%d", n), func(b *testing.B) {
			for b.Loop() {
				fs := afero.NewMemMapFs()
				const lib = "lib"
				populateEPUBs(b, fs, lib, n)
				idx := newMemBleveIndexer(b, fs, lib)
				if err := idx.AddLibrary(100, true, 0); err != nil {
					b.Fatalf("AddLibrary: %v", err)
				}
				_ = idx.Close()
			}
		})
	}
}

// BenchmarkAddLibrary_Incremental measures re-walking an already-indexed library
// (forceIndexing=false). Highlights per-path index lookups during the walk.
func BenchmarkAddLibrary_Incremental(b *testing.B) {
	sizes := []int{10, 100, 500}
	for _, n := range sizes {
		b.Run(fmt.Sprintf("epubs_%d", n), func(b *testing.B) {
			fs := afero.NewMemMapFs()
			const lib = "lib"
			populateEPUBs(b, fs, lib, n)
			idx := newMemBleveIndexer(b, fs, lib)
			if err := idx.AddLibrary(100, true, 0); err != nil {
				b.Fatalf("seed AddLibrary: %v", err)
			}
			b.ResetTimer()
			for b.Loop() {
				if err := idx.AddLibrary(100, false, 0); err != nil {
					b.Fatalf("AddLibrary: %v", err)
				}
			}
			_ = idx.Close()
		})
	}
}

// BenchmarkAddLibrary_NoiseFiles measures cost when many non-indexable files
// share a library with a few books (each path still hits the documents index).
func BenchmarkAddLibrary_NoiseFiles(b *testing.B) {
	cases := []struct {
		noise int
		epubs int
	}{
		{1000, 1},
		{5000, 1},
	}
	for _, tc := range cases {
		b.Run(fmt.Sprintf("noise_%d_epubs_%d", tc.noise, tc.epubs), func(b *testing.B) {
			for b.Loop() {
				fs := afero.NewMemMapFs()
				const lib = "lib"
				populateNoiseFiles(b, fs, lib, tc.noise)
				populateEPUBs(b, fs, lib, tc.epubs)
				idx := newMemBleveIndexer(b, fs, lib)
				if err := idx.AddLibrary(100, true, 0); err != nil {
					b.Fatalf("AddLibrary: %v", err)
				}
				_ = idx.Close()
			}
		})
	}
}

// BenchmarkAddLibrary_BatchSize compares Bleve batch flush sizes for a fixed library.
func BenchmarkAddLibrary_BatchSize(b *testing.B) {
	const n = 100
	batchSizes := []int{25, 100, 400}
	for _, bs := range batchSizes {
		b.Run(fmt.Sprintf("batch_%d", bs), func(b *testing.B) {
			for b.Loop() {
				fs := afero.NewMemMapFs()
				const lib = "lib"
				populateEPUBs(b, fs, lib, n)
				idx := newMemBleveIndexer(b, fs, lib)
				if err := idx.AddLibrary(bs, true, 0); err != nil {
					b.Fatalf("AddLibrary: %v", err)
				}
				_ = idx.Close()
			}
		})
	}
}

// BenchmarkAddLibrary_Workers compares sequential vs parallel metadata extraction (mock metadata is cheap;
// real EPUB/PDF libraries benefit more from INDEX_WORKERS > 1).
func BenchmarkAddLibrary_Workers(b *testing.B) {
	const n = 80
	for _, workers := range []int{1, 8} {
		b.Run(fmt.Sprintf("workers_%d", workers), func(b *testing.B) {
			for b.Loop() {
				fs := afero.NewMemMapFs()
				const lib = "lib"
				populateEPUBs(b, fs, lib, n)
				idx := newMemBleveIndexer(b, fs, lib)
				if err := idx.AddLibrary(100, true, workers); err != nil {
					b.Fatalf("AddLibrary: %v", err)
				}
				_ = idx.Close()
			}
		})
	}
}
