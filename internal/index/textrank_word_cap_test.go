package index_test

import (
	"fmt"
	"image"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidBelicza/TextRank/v2/rank"
	"github.com/blevesearch/bleve/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/metadata"
)

// longTextTestReader mirrors rankableTestReader but returns a text with a
// configurable word count and counts RankText calls, so tests can verify
// Config.MaxTextRankWords actually skips ranking for oversized documents
// rather than just trimming its output.
type longTextTestReader struct {
	words        int
	rankTextCall *int
}

func (r longTextTestReader) text() string {
	words := make([]string, r.words)
	for i := range words {
		words[i] = "word"
	}
	return strings.Join(words, " ")
}

func (r longTextTestReader) Metadata(path string) (metadata.Metadata, error) {
	md, _, err := r.MetadataAndText(path)
	return md, err
}

func (r longTextTestReader) MetadataAndText(path string) (metadata.Metadata, string, error) {
	return metadata.Metadata{Title: "Long Book", Authors: []string{"Author Long"}, Format: "EPUB"}, r.text(), nil
}

func (r longTextTestReader) Text(path string) (string, error) {
	return r.text(), nil
}

func (r longTextTestReader) RankText(minOccurrenceRatio float64, textContent, language string) (*metadata.TextRankResult, error) {
	if r.rankTextCall != nil {
		*r.rankTextCall++
	}
	return &metadata.TextRankResult{
		Phrases:     []rank.Phrase{{Left: "some", Right: "keyword"}},
		SingleWords: []rank.SingleWord{{Word: "standalone"}},
	}, nil
}

func (r longTextTestReader) Cover(string, int) (image.Image, error) {
	return nil, nil
}

func TestEnrichTextRankKeywordsSkipsDocumentsOverMaxTextRankWords(t *testing.T) {
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
	afero.WriteFile(appFS, "lib/long.epub", []byte(""), 0644)

	var rankCalls int
	readers := map[string]metadata.Reader{
		".epub": longTextTestReader{words: 100, rankTextCall: &rankCalls},
	}

	idx := index.NewBleve(documentsIndexMem, authorsIndexMem, appFS, "lib", readers, index.Config{
		MaxTextRankWords: 50,
	})

	if err := idx.AddLibrary(1, true, 0); err != nil {
		t.Fatalf("AddLibrary returned an error: %s", err)
	}
	if err := idx.EnrichTextRankKeywords(1, 0); err != nil {
		t.Fatalf("EnrichTextRankKeywords returned an error: %s", err)
	}

	doc, err := idx.Document("author-long-long-book")
	if err != nil {
		t.Fatalf("Document returned an error: %s", err)
	}
	if !doc.TextRankEnriched {
		t.Errorf("expected document to be marked TextRank-enriched even though ranking was skipped")
	}
	if doc.Words != 100 {
		t.Errorf("expected Words to be 100 regardless of the TextRank cap, got %v", doc.Words)
	}
	if len(doc.TextRankPhrases) != 0 || len(doc.TextRankWords) != 0 {
		t.Errorf("expected no TextRank keywords for a document over the word cap, got phrases=%q words=%q", doc.TextRankPhrases, doc.TextRankWords)
	}
	if rankCalls != 0 {
		t.Errorf("expected RankText to never be called for a document over the word cap, got %d calls", rankCalls)
	}
}

func TestEnrichTextRankKeywordsRanksDocumentsUnderMaxTextRankWords(t *testing.T) {
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
	afero.WriteFile(appFS, "lib/long.epub", []byte(""), 0644)

	var rankCalls int
	readers := map[string]metadata.Reader{
		".epub": longTextTestReader{words: 10, rankTextCall: &rankCalls},
	}

	idx := index.NewBleve(documentsIndexMem, authorsIndexMem, appFS, "lib", readers, index.Config{
		MaxTextRankWords: 50,
	})

	if err := idx.AddLibrary(1, true, 0); err != nil {
		t.Fatalf("AddLibrary returned an error: %s", err)
	}
	if err := idx.EnrichTextRankKeywords(1, 0); err != nil {
		t.Fatalf("EnrichTextRankKeywords returned an error: %s", err)
	}

	doc, err := idx.Document("author-long-long-book")
	if err != nil {
		t.Fatalf("Document returned an error: %s", err)
	}
	if len(doc.TextRankPhrases) == 0 || len(doc.TextRankWords) == 0 {
		t.Errorf("expected TextRank keywords for a document under the word cap, got phrases=%q words=%q", doc.TextRankPhrases, doc.TextRankWords)
	}
	if rankCalls != 1 {
		t.Errorf("expected RankText to be called once for a document under the word cap, got %d calls", rankCalls)
	}
}

// indexedTitleTestReader gives every path a distinct title (derived from the
// path itself) so a multi-document library doesn't collapse into slug
// collisions, and counts RankText calls like longTextTestReader.
type indexedTitleTestReader struct {
	rankTextCall *int
}

func (r indexedTitleTestReader) title(path string) string {
	return "Book " + strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

func (r indexedTitleTestReader) Metadata(path string) (metadata.Metadata, error) {
	md, _, err := r.MetadataAndText(path)
	return md, err
}

func (r indexedTitleTestReader) MetadataAndText(path string) (metadata.Metadata, string, error) {
	return metadata.Metadata{Title: r.title(path), Authors: []string{"Author Batch"}, Format: "EPUB"}, "some content about " + r.title(path), nil
}

func (r indexedTitleTestReader) Text(path string) (string, error) {
	return "some content about " + r.title(path), nil
}

func (r indexedTitleTestReader) RankText(minOccurrenceRatio float64, textContent, language string) (*metadata.TextRankResult, error) {
	if r.rankTextCall != nil {
		*r.rankTextCall++
	}
	return &metadata.TextRankResult{SingleWords: []rank.SingleWord{{Word: "keyword"}}}, nil
}

func (r indexedTitleTestReader) Cover(string, int) (image.Image, error) {
	return nil, nil
}

// TestEnrichTextRankKeywordsProcessesAllDocumentsAcrossBatches guards against
// a regression where documentsNeedingTextRank only ever hydrated the whole
// pending set once instead of paginating batchSize at a time: it processed a
// library that's larger than one batch and expects every document to end up
// enriched, not just the first batchSize of them.
func TestEnrichTextRankKeywordsProcessesAllDocumentsAcrossBatches(t *testing.T) {
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

	const docCount = 5
	for i := 0; i < docCount; i++ {
		afero.WriteFile(appFS, fmt.Sprintf("lib/book%d.epub", i), []byte(""), 0644)
	}

	var rankCalls int
	readers := map[string]metadata.Reader{
		".epub": indexedTitleTestReader{rankTextCall: &rankCalls},
	}

	idx := index.NewBleve(documentsIndexMem, authorsIndexMem, appFS, "lib", readers, index.Config{})

	if err := idx.AddLibrary(2, true, 0); err != nil {
		t.Fatalf("AddLibrary returned an error: %s", err)
	}
	// batchSize=2 with docCount=5 forces documentsNeedingTextRank to be
	// called more than once.
	if err := idx.EnrichTextRankKeywords(2, 0); err != nil {
		t.Fatalf("EnrichTextRankKeywords returned an error: %s", err)
	}

	results, err := idx.Search(index.SearchFields{}, 1, docCount)
	if err != nil {
		t.Fatalf("Search returned an error: %s", err)
	}
	docs := results.Hits()
	if len(docs) != docCount {
		t.Fatalf("expected %d documents, got %d", docCount, len(docs))
	}
	for _, doc := range docs {
		if !doc.TextRankEnriched {
			t.Errorf("expected document %q to be TextRank-enriched after EnrichTextRankKeywords", doc.ID)
		}
		if len(doc.TextRankWords) == 0 {
			t.Errorf("expected document %q to have TextRank keywords after EnrichTextRankKeywords", doc.ID)
		}
	}
	if rankCalls != docCount {
		t.Errorf("expected RankText to be called exactly once per document (%d), got %d calls", docCount, rankCalls)
	}
}
