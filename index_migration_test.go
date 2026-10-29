package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/metadata"
)

func TestGetIndexesMigratesAuthorsWithoutReindexingDocuments(t *testing.T) {
	previousHome, previousInput := homeDir, input
	t.Cleanup(func() { homeDir, input = previousHome, previousInput })
	homeDir = t.TempDir()
	input.BatchSize = 1
	fs := afero.NewOsFs()
	if err := fs.MkdirAll(filepath.Join(homeDir, ".coreander"), 0755); err != nil {
		t.Fatal(err)
	}
	documents := index.CreateDocumentsIndex(homeDir + documentsIndexPath)
	for key, value := range map[string]string{
		"version":              index.DocumentVersion,
		"illustrated_min_size": "0.25",
		"min_occurrence_ratio": "0.1",
		"max_textrank_words":   "10000",
	} {
		if err := documents.SetInternal([]byte(key), []byte(value)); err != nil {
			t.Fatal(err)
		}
	}
	doc := index.Document{
		ID: "book", Slug: "book", AuthorsSlugs: []string{"alice-borchardt"},
		Metadata:          metadata.Metadata{Title: "La Reina Dragon", Authors: []string{"Alice Borchardt"}},
		GoodReadsEnriched: true,
	}
	if err := documents.Index(doc.ID, doc); err != nil {
		t.Fatal(err)
	}
	if err := documents.Close(); err != nil {
		t.Fatal(err)
	}
	legacyMapping := index.CreateAuthorsMapping().(*mapping.IndexMappingImpl)
	delete(legacyMapping.DefaultMapping.Properties, "GoodReadsAuthorURL")
	authors, err := bleve.New(homeDir+authorsIndexPath, legacyMapping)
	if err != nil {
		t.Fatal(err)
	}
	retrieved := time.Now().UTC().Truncate(time.Second)
	if err := authors.Index("alice-borchardt", index.Author{
		Slug: "alice-borchardt", Name: "Alice Borchardt", DataSourceID: "Q1", RetrievedOn: retrieved,
	}); err != nil {
		t.Fatal(err)
	}
	if err := authors.SetInternal([]byte("version"), []byte("2")); err != nil {
		t.Fatal(err)
	}
	if err := authors.Close(); err != nil {
		t.Fatal(err)
	}

	documents, authors, needsReindex := getIndexes(fs, 0.25, 0.1, 10000)
	idx := index.NewBleve(documents, authors, nil, "", nil, index.Config{})
	defer idx.Close()
	if needsReindex {
		t.Fatal("author migration must not require document reindexing")
	}
	if count, err := documents.DocCount(); err != nil || count != 1 {
		t.Fatalf("document index was not preserved: count=%d, err=%v", count, err)
	}
	author, err := idx.Author("alice-borchardt", "")
	if err != nil || author.DataSourceID != "Q1" || !author.RetrievedOn.Equal(retrieved) {
		t.Fatalf("expected existing author metadata preserved: %+v, %v", author, err)
	}
}
