package index_test

import (
	"fmt"
	"image"
	"path/filepath"
	"slices"
	"strings"
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
// TextRankPhrases/TextRankWords.
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

func (rankableTestReader) RankText(minOccurrenceRatio float64, textContent, filename string) (*metadata.TextRankResult, error) {
	return &metadata.TextRankResult{
		Phrases:     []rank.Phrase{{Left: "some", Right: "keyword"}},
		SingleWords: []rank.SingleWord{{Word: "standalone"}},
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
	if len(rankable.TextRankPhrases) != 0 {
		t.Errorf("expected rankable document to have no TextRankPhrases yet, got %q", rankable.TextRankPhrases)
	}
	if len(rankable.TextRankWords) != 0 {
		t.Errorf("expected rankable document to have no TextRankWords yet, got %q", rankable.TextRankWords)
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
	wantPhrases := []string{"some keyword"}
	if !slices.Equal(rankable.TextRankPhrases, wantPhrases) {
		t.Errorf("expected rankable document to have TextRankPhrases %q, got %q", wantPhrases, rankable.TextRankPhrases)
	}
	wantWords := []string{"standalone"}
	if !slices.Equal(rankable.TextRankWords, wantWords) {
		t.Errorf("expected rankable document to have TextRankWords %q, got %q", wantWords, rankable.TextRankWords)
	}

	nonRankable, err = idx.Document("author-two-non-rankable-book")
	if err != nil {
		t.Fatalf("Document returned an error: %s", err)
	}
	if !nonRankable.TextRankEnriched {
		t.Errorf("expected non-rankable document to remain TextRank-enriched")
	}
}

// countingTestReader mirrors rankableTestReader but with a configurable
// title/author/content, so it can stand in for both a single long document
// (fixed title, word count driving text length) and a batch of distinct
// documents (title derived per path, fixed short content). It counts
// RankText calls and, if receivedWordCount is set, records the word count of
// the textContent it actually receives, so tests can verify
// Config.MaxTextRankWords truncates the text fed to TextRank for oversized
// documents rather than skipping ranking outright.
type countingTestReader struct {
	title             string // fixed title; if empty, derived from path
	author            string
	words             int // if > 0, text() returns this many "word" tokens instead of title-based content
	rankTextCall      *int
	receivedWordCount *int
}

func (r countingTestReader) titleFor(path string) string {
	if r.title != "" {
		return r.title
	}
	return "Book " + strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

func (r countingTestReader) text(path string) string {
	if r.words > 0 {
		words := make([]string, r.words)
		for i := range words {
			words[i] = "word"
		}
		return strings.Join(words, " ")
	}
	return "some content about " + r.titleFor(path)
}

func (r countingTestReader) Metadata(path string) (metadata.Metadata, error) {
	md, _, err := r.MetadataAndText(path)
	return md, err
}

func (r countingTestReader) MetadataAndText(path string) (metadata.Metadata, string, error) {
	return metadata.Metadata{Title: r.titleFor(path), Authors: []string{r.author}, Format: "EPUB"}, r.text(path), nil
}

func (r countingTestReader) Text(path string) (string, error) {
	return r.text(path), nil
}

func (r countingTestReader) RankText(minOccurrenceRatio float64, textContent, language string) (*metadata.TextRankResult, error) {
	if r.rankTextCall != nil {
		*r.rankTextCall++
	}
	if r.receivedWordCount != nil {
		*r.receivedWordCount = len(strings.Fields(textContent))
	}
	return &metadata.TextRankResult{
		Phrases:     []rank.Phrase{{Left: "some", Right: "keyword"}},
		SingleWords: []rank.SingleWord{{Word: "standalone"}},
	}, nil
}

func (r countingTestReader) Cover(string, int) (image.Image, error) {
	return nil, nil
}

func TestEnrichTextRankKeywordsRespectsMaxTextRankWords(t *testing.T) {
	tests := []struct {
		name              string
		words             int
		wantReceivedWords int
	}{
		{name: "under the word cap, ranked as-is", words: 10, wantReceivedWords: 10},
		{name: "over the word cap, truncated before ranking", words: 100, wantReceivedWords: 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			var rankCalls, receivedWordCount int
			readers := map[string]metadata.Reader{
				".epub": countingTestReader{title: "Long Book", author: "Author Long", words: tt.words, rankTextCall: &rankCalls, receivedWordCount: &receivedWordCount},
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
				t.Errorf("expected document to be marked TextRank-enriched")
			}
			if doc.Words != float64(tt.words) {
				t.Errorf("expected Words to be %d (the whole document), regardless of the TextRank cap, got %v", tt.words, doc.Words)
			}
			if len(doc.TextRankPhrases) == 0 || len(doc.TextRankWords) == 0 {
				t.Errorf("expected TextRank keywords for the document, since it should still be analyzed (possibly truncated), got phrases=%q words=%q", doc.TextRankPhrases, doc.TextRankWords)
			}
			if rankCalls != 1 {
				t.Errorf("expected RankText to be called exactly once, got %d calls", rankCalls)
			}
			if receivedWordCount != tt.wantReceivedWords {
				t.Errorf("expected RankText to receive %d words, got %d", tt.wantReceivedWords, receivedWordCount)
			}
		})
	}
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
		".epub": countingTestReader{author: "Author Batch", rankTextCall: &rankCalls},
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
