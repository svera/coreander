package index_test

import (
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/metadata"
)

func TestDocumentCountsByAuthorSlugs(t *testing.T) {
	documentsIndexMem, err := bleve.NewMemOnly(index.CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}
	authorsIndexMem, err := bleve.NewMemOnly(index.CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}

	idx := index.NewBleve(documentsIndexMem, authorsIndexMem, nil, "", nil, index.Config{})

	docs := []index.Document{
		{
			ID:           "doc-1",
			Slug:         "doc-1",
			AuthorsSlugs: []string{"george-orwell"},
			Metadata:     metadata.Metadata{Title: "1984", Authors: []string{"George Orwell"}},
		},
		{
			ID:           "doc-2",
			Slug:         "doc-2",
			AuthorsSlugs: []string{"george-orwell"},
			Metadata:     metadata.Metadata{Title: "Animal Farm", Authors: []string{"George Orwell"}},
		},
		{
			ID:                "doc-3",
			Slug:              "doc-3",
			IllustratorsSlugs: []string{"jane-austen"},
			Metadata:          metadata.Metadata{Title: "Illustrated", Illustrators: []string{"Jane Austen"}},
		},
	}
	for _, doc := range docs {
		if err := documentsIndexMem.Index(doc.ID, doc); err != nil {
			t.Fatal(err)
		}
	}

	counts, err := idx.DocumentCountsByAuthorSlugs([]string{"george-orwell", "jane-austen", "unknown"})
	if err != nil {
		t.Fatal(err)
	}
	if counts["george-orwell"] != 2 {
		t.Fatalf("expected 2 documents for George Orwell, got %d", counts["george-orwell"])
	}
	if counts["jane-austen"] != 1 {
		t.Fatalf("expected 1 document for Jane Austen, got %d", counts["jane-austen"])
	}
	if counts["unknown"] != 0 {
		t.Fatalf("expected 0 documents for unknown author, got %d", counts["unknown"])
	}
}
