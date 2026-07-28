package index_test

import (
	"image"
	"testing"

	"github.com/DavidBelicza/TextRank/v2/rank"
	"github.com/blevesearch/bleve/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/metadata"
)

// rankableTestReader implements metadata.TextExtractor, metadata.TextSource
// and metadata.TextRanker, mirroring the real EpubReader, so AddLibrary defers
// TextRank analysis and EnrichTextRankKeywords is required to populate
// TextRankKeywords.
type rankableTestReader struct{}

func (rankableTestReader) Metadata(path string) (metadata.Metadata, error) {
	md, _, err := rankableTestReader{}.MetadataAndText(path)
	return md, err
}

func (rankableTestReader) MetadataAndText(path string) (metadata.Metadata, string, error) {
	return metadata.Metadata{Title: "Rankable Book", Authors: []string{"Author One"}, Format: "EPUB"}, "some content", nil
}

func (rankableTestReader) Text(path string) (string, error) {
	return "some content", nil
}

func (rankableTestReader) RankText(textContent, filename string) (*metadata.TextRankResult, error) {
	return &metadata.TextRankResult{
		SingleWords: []rank.SingleWord{{Word: "keyword"}},
	}, nil
}

func (rankableTestReader) Cover(string, int) (image.Image, error) {
	return nil, nil
}

// nonRankableTestReader implements only metadata.Reader, mirroring PdfReader,
// which has no TextRank support at all.
type nonRankableTestReader struct{}

func (nonRankableTestReader) Metadata(path string) (metadata.Metadata, error) {
	return metadata.Metadata{Title: "Non-rankable Book", Authors: []string{"Author Two"}, Format: "PDF"}, nil
}

func (nonRankableTestReader) Cover(string, int) (image.Image, error) {
	return nil, nil
}

func TestAddLibraryDefersTextRankToEnrichment(t *testing.T) {
	documentsIndexMem, err := bleve.NewMemOnly(index.CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}
	authorsIndexMem, err := bleve.NewMemOnly(index.CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}

	appFS := afero.NewMemMapFs()
	appFS.MkdirAll("lib", 0755)
	afero.WriteFile(appFS, "lib/rankable.epub", []byte(""), 0644)
	afero.WriteFile(appFS, "lib/nonrankable.pdf", []byte(""), 0644)

	readers := map[string]metadata.Reader{
		".epub": rankableTestReader{},
		".pdf":  nonRankableTestReader{},
	}

	idx := index.NewBleve(documentsIndexMem, authorsIndexMem, appFS, "lib", readers, index.Config{})

	if err := idx.AddLibrary(1, true, 0); err != nil {
		t.Fatalf("AddLibrary returned an error: %s", err)
	}

	rankable, err := idx.Document("author-one-rankable-book")
	if err != nil {
		t.Fatalf("Document returned an error: %s", err)
	}
	if rankable.TextRankEnriched {
		t.Errorf("expected rankable document to not be TextRank-enriched right after AddLibrary")
	}
	if rankable.TextRankKeywords != "" {
		t.Errorf("expected rankable document to have no TextRankKeywords yet, got %q", rankable.TextRankKeywords)
	}

	nonRankable, err := idx.Document("author-two-non-rankable-book")
	if err != nil {
		t.Fatalf("Document returned an error: %s", err)
	}
	if !nonRankable.TextRankEnriched {
		t.Errorf("expected non-rankable document to be immediately marked as TextRank-enriched, since it has no TextRank support")
	}

	if err := idx.EnrichTextRankKeywords(1, 0); err != nil {
		t.Fatalf("EnrichTextRankKeywords returned an error: %s", err)
	}

	rankable, err = idx.Document("author-one-rankable-book")
	if err != nil {
		t.Fatalf("Document returned an error: %s", err)
	}
	if !rankable.TextRankEnriched {
		t.Errorf("expected rankable document to be TextRank-enriched after EnrichTextRankKeywords")
	}
	if rankable.TextRankKeywords != "keyword" {
		t.Errorf("expected rankable document to have TextRankKeywords %q, got %q", "keyword", rankable.TextRankKeywords)
	}

	nonRankable, err = idx.Document("author-two-non-rankable-book")
	if err != nil {
		t.Fatalf("Document returned an error: %s", err)
	}
	if !nonRankable.TextRankEnriched {
		t.Errorf("expected non-rankable document to remain TextRank-enriched")
	}
}
