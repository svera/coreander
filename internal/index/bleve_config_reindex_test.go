package index

import (
	"strconv"
	"testing"

	"github.com/blevesearch/bleve/v2"
)

func TestNeedsReindexWhenMaxTextRankWordsChanges(t *testing.T) {
	documentsIndex, err := bleve.NewMemOnly(CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}

	for key, value := range map[string]string{
		string(internalIllustratedMinSize): "0.25",
		string(internalMinOccurrenceRatio): "0.1",
		string(internalMaxTextRankWords):   strconv.Itoa(100),
	} {
		if err := documentsIndex.SetInternal([]byte(key), []byte(value)); err != nil {
			t.Fatal(err)
		}
	}

	needsReindex, err := NeedsReindex(documentsIndex, 0.25, 0.1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if needsReindex {
		t.Fatal("expected unchanged TextRank word limit not to require reindexing")
	}

	needsReindex, err = NeedsReindex(documentsIndex, 0.25, 0.1, 200)
	if err != nil {
		t.Fatal(err)
	}
	if !needsReindex {
		t.Fatal("expected changed TextRank word limit to require reindexing")
	}
}

func TestNeedsReindexWhenMaxTextRankWordsSettingIsMissing(t *testing.T) {
	documentsIndex, err := bleve.NewMemOnly(CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}

	if err := documentsIndex.SetInternal(internalIllustratedMinSize, []byte("0.25")); err != nil {
		t.Fatal(err)
	}
	if err := documentsIndex.SetInternal(internalMinOccurrenceRatio, []byte("0.1")); err != nil {
		t.Fatal(err)
	}

	needsReindex, err := NeedsReindex(documentsIndex, 0.25, 0.1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !needsReindex {
		t.Fatal("expected missing TextRank word limit to require reindexing")
	}
}
