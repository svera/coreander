package index_test

import (
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/metadata"
)

func TestRebuildAuthorsFromDocuments(t *testing.T) {
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
			ID:                "doc-2",
			Slug:              "doc-2",
			IllustratorsSlugs: []string{"jane-austen"},
			Metadata:          metadata.Metadata{Title: "Illustrated", Illustrators: []string{"Jane Austen"}},
		},
	}
	for _, doc := range docs {
		if err := documentsIndexMem.Index(doc.ID, doc); err != nil {
			t.Fatal(err)
		}
	}

	if count, err := authorsIndexMem.DocCount(); err != nil {
		t.Fatal(err)
	} else if count != 0 {
		t.Fatalf("expected empty authors index, got %d", count)
	}

	if err := idx.RebuildAuthorsFromDocuments(10); err != nil {
		t.Fatal(err)
	}

	if count, err := authorsIndexMem.DocCount(); err != nil {
		t.Fatal(err)
	} else if count != 2 {
		t.Fatalf("expected 2 authors after rebuild, got %d", count)
	}

	author, err := idx.Author("george-orwell", "en")
	if err != nil || author.Name != "George Orwell" {
		t.Fatalf("expected George Orwell, got %#v err=%v", author, err)
	}
	if author.DocumentCount != 1 {
		t.Fatalf("expected DocumentCount=1 for George Orwell, got %d", author.DocumentCount)
	}

	illustrator, err := idx.Author("jane-austen", "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if illustrator.DocumentCount != 1 {
		t.Fatalf("expected DocumentCount=1 for Jane Austen, got %d", illustrator.DocumentCount)
	}
}
