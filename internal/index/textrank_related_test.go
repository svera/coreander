package index_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/metadata"
	"github.com/svera/coreander/v5/internal/precisiondate"
)

// newTextRankTestIndex builds an in-memory documents index (bypassing AddLibrary
// and metadata.Reader entirely, mirroring TestRebuildAuthorsFromDocuments) and
// indexes docs directly, so TextRankKeywords can be set explicitly without
// needing a real EPUB file and EpubReader.RankText run.
func newTextRankTestIndex(t *testing.T, docs []index.Document) *index.BleveIndexer {
	t.Helper()
	return newTextRankTestIndexWithConfig(t, docs, index.Config{})
}

func newTextRankTestIndexWithConfig(t *testing.T, docs []index.Document, cfg index.Config) *index.BleveIndexer {
	t.Helper()

	documentsIndexMem, err := bleve.NewMemOnly(index.CreateDocumentsMapping())
	if err != nil {
		t.Fatal(err)
	}
	authorsIndexMem, err := bleve.NewMemOnly(index.CreateAuthorsMapping())
	if err != nil {
		t.Fatal(err)
	}

	idx := index.NewBleve(documentsIndexMem, authorsIndexMem, nil, "", nil, cfg)

	for _, doc := range docs {
		if err := documentsIndexMem.Index(doc.ID, doc); err != nil {
			t.Fatal(err)
		}
	}

	return idx
}

func slugsOf(docs []index.Document) []string {
	slugs := make([]string, len(docs))
	for i, d := range docs {
		slugs[i] = d.Slug
	}
	return slugs
}

// TestSimilarToMatchesBySharedTextRankPhrasesOnly guards SimilarTo's
// contract: it delegates to Search's SearchFields.SimilarTo path
// (similarToQuery), so it matches purely on shared TextRankPhrases and never
// considers Subjects at all.
func TestSimilarToMatchesBySharedTextRankPhrasesOnly(t *testing.T) {
	sharedPairs := []string{"oppenheimer manhattan", "atomic bomb", "los alamos"}

	docWithPhrases := index.Document{
		ID:              "with-phrases.epub",
		Slug:            "with-phrases",
		Metadata:        metadata.Metadata{Title: "With Phrases", Authors: []string{"Author One"}, Format: "EPUB"},
		AuthorsSlugs:    []string{"author-one"},
		TextRankPhrases: sharedPairs,
	}
	docSharedPhrases := index.Document{
		ID:              "shared-phrases.epub",
		Slug:            "shared-phrases",
		Metadata:        metadata.Metadata{Title: "Shared Phrases", Authors: []string{"Author Two"}, Format: "EPUB"},
		AuthorsSlugs:    []string{"author-two"},
		TextRankPhrases: sharedPairs,
	}
	// docSharedSubjectOnly shares a subject with docWithPhrases but no
	// TextRank phrase, so it must not appear in SimilarTo's results.
	docSharedSubjectOnly := index.Document{
		ID:            "shared-subject.epub",
		Slug:          "shared-subject",
		Metadata:      metadata.Metadata{Title: "Shared Subject", Authors: []string{"Author Three"}, Format: "EPUB", Subjects: []string{"History"}},
		AuthorsSlugs:  []string{"author-three"},
		SubjectsSlugs: []string{"history"},
	}
	docUnrelated := index.Document{
		ID:              "unrelated.epub",
		Slug:            "unrelated",
		Metadata:        metadata.Metadata{Title: "Unrelated", Authors: []string{"Author Four"}, Format: "EPUB"},
		AuthorsSlugs:    []string{"author-four"},
		TextRankPhrases: []string{"cooking recipes", "bread baking", "gardening tools"},
	}
	docSameAuthor := index.Document{
		ID:              "same-author.epub",
		Slug:            "same-author",
		Metadata:        metadata.Metadata{Title: "Same Author", Authors: []string{"Author One"}, Format: "EPUB"},
		AuthorsSlugs:    []string{"author-one"},
		TextRankPhrases: sharedPairs,
	}

	idx := newTextRankTestIndex(t, []index.Document{
		docWithPhrases, docSharedPhrases, docSharedSubjectOnly, docUnrelated, docSameAuthor,
	})

	got, err := idx.SimilarTo("with-phrases", 10)
	if err != nil {
		t.Fatalf("SimilarTo returned an error: %s", err)
	}

	gotSlugs := slugsOf(got)
	if !slices.Contains(gotSlugs, "shared-phrases") {
		t.Errorf("expected %q to be present in results %v", "shared-phrases", gotSlugs)
	}
	if slices.Contains(gotSlugs, "shared-subject") {
		t.Errorf("did not expect a document sharing only a subject (no TextRank phrase) to match, got %v", gotSlugs)
	}
	if slices.Contains(gotSlugs, "unrelated") {
		t.Errorf("did not expect unrelated document to match, got %v", gotSlugs)
	}
	if slices.Contains(gotSlugs, "same-author") {
		t.Errorf("did not expect same-author document to match (SimilarTo excludes same-author documents), got %v", gotSlugs)
	}
}

// TestSimilarToWidgetUsesDefaultSortOrder guards against a regression where
// the widget's SearchFields left SortBy unset: runSimilarityQuery only
// applies the "-_score" comparator sortSimilarityResults sorts by, so with a
// nil SortBy it fell back to sorting purely by closeness to the reference
// document's publication date, ignoring how well each candidate actually
// matched. That could rank a weakly-matching but date-close document above a
// strongly-matching one, producing a different top N than the "similar"
// search, which always sets SortBy (via parseDocumentSortBy, defaulting to
// DefaultDocumentSortBy) - see TestSimilarToWidgetMatchesSeeAllSearch for the
// consistency guard this complements.
func TestSimilarToWidgetUsesDefaultSortOrder(t *testing.T) {
	docA := index.Document{
		ID:              "a.epub",
		Slug:            "doc-a",
		Metadata:        metadata.Metadata{Title: "Doc A", Authors: []string{"Author One"}, Format: "EPUB", Publication: precisiondate.NewPrecisionDate("2020-01-01T00:00:00Z", precisiondate.PrecisionDay)},
		AuthorsSlugs:    []string{"author-one"},
		TextRankPhrases: []string{"oppenheimer manhattan", "project atomic", "los alamos", "physics nuclear"},
	}
	// docStrongMatch shares every TextRank phrase with docA (highest score),
	// but its publication date is far from docA's.
	docStrongMatch := index.Document{
		ID:              "strong.epub",
		Slug:            "strong-match",
		Metadata:        metadata.Metadata{Title: "Strong Match", Authors: []string{"Author Two"}, Format: "EPUB", Publication: precisiondate.NewPrecisionDate("1990-01-01T00:00:00Z", precisiondate.PrecisionDay)},
		AuthorsSlugs:    []string{"author-two"},
		TextRankPhrases: []string{"oppenheimer manhattan", "project atomic", "los alamos", "physics nuclear"},
	}
	// docWeakMatch shares only 3 of docA's 4 TextRank phrases (lower score,
	// but still within MinSimilarityScoreRatio of docStrongMatch's, so it
	// isn't pruned), and its publication date is right next to docA's - if
	// sorting fell back to date-closeness alone, this would rank above
	// docStrongMatch.
	docWeakMatch := index.Document{
		ID:              "weak.epub",
		Slug:            "weak-match",
		Metadata:        metadata.Metadata{Title: "Weak Match", Authors: []string{"Author Three"}, Format: "EPUB", Publication: precisiondate.NewPrecisionDate("2020-01-02T00:00:00Z", precisiondate.PrecisionDay)},
		AuthorsSlugs:    []string{"author-three"},
		TextRankPhrases: []string{"oppenheimer manhattan", "project atomic", "los alamos"},
	}

	idx := newTextRankTestIndex(t, []index.Document{docA, docStrongMatch, docWeakMatch})

	widgetResults, err := idx.SimilarTo("doc-a", 1)
	if err != nil {
		t.Fatalf("SimilarTo returned an error: %s", err)
	}

	widgetSlugs := slugsOf(widgetResults)
	if !slices.Contains(widgetSlugs, "strong-match") {
		t.Errorf("expected the top widget result to be the strongest TextRank match regardless of publication date, got %v", widgetSlugs)
	}
}

// TestSearchSimilarToIgnoresSingleWords guards against a real bug: a single word
// can rank highly within one document's own TextRank weights while still
// being a generic word that appears across a large fraction of an unrelated
// library (e.g. "house", "life"), which used to pull thousands of unrelated
// documents into the candidate pool for a single reference book. Two-word
// pairs don't have this problem - sharing both words of a pair with another
// document is a much stronger, more specific signal - so similarToQuery reads
// only Document.TextRankPhrases, never Document.TextRankWords, for
// similarity matching.
func TestSearchSimilarToIgnoresSingleWords(t *testing.T) {
	docA := index.Document{
		ID:              "a.epub",
		Slug:            "doc-a",
		Metadata:        metadata.Metadata{Title: "Doc A", Authors: []string{"Author One"}, Format: "EPUB"},
		AuthorsSlugs:    []string{"author-one"},
		TextRankWords:   []string{"house"},
		TextRankPhrases: []string{"oppenheimer manhattan"},
	}
	docSharesOnlySingleWord := index.Document{
		ID:            "single-word.epub",
		Slug:          "single-word",
		Metadata:      metadata.Metadata{Title: "Single Word", Authors: []string{"Author Two"}, Format: "EPUB"},
		AuthorsSlugs:  []string{"author-two"},
		TextRankWords: []string{"house", "cooking", "bread"},
	}
	docSharesPair := index.Document{
		ID:              "shared-pair.epub",
		Slug:            "shared-pair",
		Metadata:        metadata.Metadata{Title: "Shared Pair", Authors: []string{"Author Three"}, Format: "EPUB"},
		AuthorsSlugs:    []string{"author-three"},
		TextRankPhrases: []string{"oppenheimer manhattan", "cooking recipes"},
	}

	idx := newTextRankTestIndex(t, []index.Document{docA, docSharesOnlySingleWord, docSharesPair})

	res, err := idx.Search(index.SearchFields{SimilarTo: "doc-a"}, 1, 10)
	if err != nil {
		t.Fatalf("Search returned an error: %s", err)
	}

	gotSlugs := slugsOf(res.Hits())
	if slices.Contains(gotSlugs, "single-word") {
		t.Errorf("did not expect a document sharing only a single word to match, got %v", gotSlugs)
	}
	if !slices.Contains(gotSlugs, "shared-pair") {
		t.Errorf("expected a document sharing a two-word pair to match, got %v", gotSlugs)
	}
}

// TestSimilarToWidgetMatchesSeeAllSearch guards that the document detail
// page's "related documents" widget (BleveIndexer.SimilarTo) and its "See
// all" link (Search's SearchFields.SimilarTo) return the same documents,
// since SimilarTo delegates to Search. Also guards against a document that
// shares only a subject (no TextRank phrase) with the reference document
// showing up in either: a shared subject slug (e.g. a broad genre like
// "History") is too weak/generic a signal for content similarity - see
// similarToQuery.
func TestSimilarToWidgetMatchesSeeAllSearch(t *testing.T) {
	docA := index.Document{
		ID:              "a.epub",
		Slug:            "doc-a",
		Metadata:        metadata.Metadata{Title: "Doc A", Authors: []string{"Author One"}, Format: "EPUB", Subjects: []string{"History", "Physics"}},
		AuthorsSlugs:    []string{"author-one"},
		SubjectsSlugs:   []string{"history", "physics"},
		TextRankPhrases: []string{"oppenheimer manhattan", "project atomic", "los alamos", "physics nuclear"},
	}
	docSameSubject := index.Document{
		ID:            "same-subject.epub",
		Slug:          "same-subject",
		Metadata:      metadata.Metadata{Title: "Same Subject", Authors: []string{"Author Two"}, Format: "EPUB", Subjects: []string{"History", "Physics"}},
		AuthorsSlugs:  []string{"author-two"},
		SubjectsSlugs: []string{"history", "physics"},
	}
	docSharedKeywords := index.Document{
		ID:              "shared-keywords.epub",
		Slug:            "shared-keywords",
		Metadata:        metadata.Metadata{Title: "Shared Keywords", Authors: []string{"Author Three"}, Format: "EPUB"},
		AuthorsSlugs:    []string{"author-three"},
		TextRankPhrases: []string{"oppenheimer manhattan", "project atomic", "los alamos"},
	}
	docUnrelated := index.Document{
		ID:              "unrelated.epub",
		Slug:            "unrelated",
		Metadata:        metadata.Metadata{Title: "Unrelated", Authors: []string{"Author Four"}, Format: "EPUB"},
		AuthorsSlugs:    []string{"author-four"},
		TextRankPhrases: []string{"cooking recipes", "bread baking", "gardening tools"},
	}

	idx := newTextRankTestIndex(t, []index.Document{docA, docSameSubject, docSharedKeywords, docUnrelated})

	widgetResults, err := idx.SimilarTo("doc-a", 4)
	if err != nil {
		t.Fatalf("SimilarTo returned an error: %s", err)
	}

	seeAllResults, err := idx.Search(index.SearchFields{SimilarTo: "doc-a"}, 1, 10)
	if err != nil {
		t.Fatalf("Search returned an error: %s", err)
	}

	widgetSlugs := slugsOf(widgetResults)
	seeAllSlugs := slugsOf(seeAllResults.Hits())

	if slices.Contains(widgetSlugs, "same-subject") {
		t.Errorf("did not expect the SimilarTo widget to match on shared subjects alone, got %v", widgetSlugs)
	}
	if slices.Contains(seeAllSlugs, "same-subject") {
		t.Errorf("did not expect the \"similar\" search to match on shared subjects alone, got %v", seeAllSlugs)
	}
	if !slices.Contains(widgetSlugs, "shared-keywords") {
		t.Errorf("expected the widget to match on shared TextRank keywords, got %v", widgetSlugs)
	}
	if !slices.Contains(seeAllSlugs, "shared-keywords") {
		t.Errorf("expected the \"similar\" search to still match on shared TextRank keywords, got %v", seeAllSlugs)
	}
	if slices.Contains(widgetSlugs, "unrelated") {
		t.Errorf("did not expect unrelated document to match, got %v", widgetSlugs)
	}
	if slices.Contains(seeAllSlugs, "unrelated") {
		t.Errorf("did not expect unrelated document to match, got %v", seeAllSlugs)
	}
}

func TestSearchSimilarToPrunesWeakMatches(t *testing.T) {
	docA := index.Document{
		ID:              "a.epub",
		Slug:            "doc-a",
		Metadata:        metadata.Metadata{Title: "Doc A", Authors: []string{"Author One"}, Format: "EPUB"},
		AuthorsSlugs:    []string{"author-one"},
		TextRankPhrases: []string{"oppenheimer manhattan", "project atomic", "los alamos", "physics nuclear"},
	}
	docStrongMatch := index.Document{
		ID:              "strong.epub",
		Slug:            "strong-match",
		Metadata:        metadata.Metadata{Title: "Strong Match", Authors: []string{"Author Two"}, Format: "EPUB"},
		AuthorsSlugs:    []string{"author-two"},
		TextRankPhrases: []string{"oppenheimer manhattan", "project atomic", "los alamos"},
	}
	docWeakMatch := index.Document{
		ID:           "weak.epub",
		Slug:         "weak-match",
		Metadata:     metadata.Metadata{Title: "Weak Match", Authors: []string{"Author Three"}, Format: "EPUB"},
		AuthorsSlugs: []string{"author-three"},
		// Shares only one pair ("physics nuclear") with doc-a - should score
		// far below the strong match and get pruned by the similarity
		// threshold.
		TextRankPhrases: []string{"physics nuclear", "cooking recipes", "bread baking", "gardening tools"},
	}
	docUnrelated := index.Document{
		ID:              "unrelated.epub",
		Slug:            "unrelated",
		Metadata:        metadata.Metadata{Title: "Unrelated", Authors: []string{"Author Four"}, Format: "EPUB"},
		AuthorsSlugs:    []string{"author-four"},
		TextRankPhrases: []string{"completely unrelated", "cooking recipes", "bread baking"},
	}

	idx := newTextRankTestIndex(t, []index.Document{docA, docStrongMatch, docWeakMatch, docUnrelated})

	res, err := idx.Search(index.SearchFields{SimilarTo: "doc-a"}, 1, 10)
	if err != nil {
		t.Fatalf("Search returned an error: %s", err)
	}

	gotSlugs := slugsOf(res.Hits())
	if !slices.Contains(gotSlugs, "strong-match") {
		t.Errorf("expected strong-match to be present, got %v", gotSlugs)
	}
	if slices.Contains(gotSlugs, "weak-match") {
		t.Errorf("expected weak-match to be pruned by the similarity threshold, got %v", gotSlugs)
	}
	if slices.Contains(gotSlugs, "unrelated") {
		t.Errorf("did not expect unrelated document to match, got %v", gotSlugs)
	}
}

// TestSearchSimilarToReportsCappedCandidates checks that when a "similar
// document" query has more matches than MaxSimilarityCandidates, the
// resulting CappedPaginatedResult reports the cap was hit (Candidates.Capped)
// along with the true, uncapped match count (Candidates.Total) - so a caller
// (e.g. the search UI's "similar to" banner) can tell the user some matches
// weren't even considered, rather than silently dropping them.
func TestSearchSimilarToReportsCappedCandidates(t *testing.T) {
	sharedKeywords := []string{"oppenheimer manhattan", "project atomic", "los alamos"}

	docA := index.Document{
		ID:              "a.epub",
		Slug:            "doc-a",
		Metadata:        metadata.Metadata{Title: "Doc A", Authors: []string{"Author One"}, Format: "EPUB"},
		AuthorsSlugs:    []string{"author-one"},
		TextRankPhrases: sharedKeywords,
	}
	docs := []index.Document{docA}
	for i := 1; i <= 4; i++ {
		docs = append(docs, index.Document{
			ID:              fmt.Sprintf("match%d.epub", i),
			Slug:            fmt.Sprintf("match-%d", i),
			Metadata:        metadata.Metadata{Title: fmt.Sprintf("Match %d", i), Authors: []string{fmt.Sprintf("Author %d", i)}, Format: "EPUB"},
			AuthorsSlugs:    []string{fmt.Sprintf("author-%d", i)},
			TextRankPhrases: sharedKeywords,
		})
	}

	idx := newTextRankTestIndexWithConfig(t, docs, index.Config{
		MaxSimilarityCandidates: 2,
		MinSimilarityScoreRatio: 0.01,
	})

	res, err := idx.Search(index.SearchFields{SimilarTo: "doc-a"}, 1, 10)
	if err != nil {
		t.Fatalf("Search returned an error: %s", err)
	}

	if !res.Candidates.Capped() {
		t.Errorf("expected Candidates.Capped to report true with 4 matches and a cap of 2, got false (Total=%d, Cap=%d)", res.Candidates.Total(), res.Candidates.Cap())
	}
	if res.Candidates.Total() != 4 {
		t.Errorf("expected Candidates.Total to report the uncapped match count 4, got %d", res.Candidates.Total())
	}
	if res.Candidates.Cap() != 2 {
		t.Errorf("expected Candidates.Cap to report the configured cap 2, got %d", res.Candidates.Cap())
	}
}

// TestSearchSimilarToDoesNotReportCappedWhenThresholdIsTheLimit checks that
// Candidates.Capped reports false when the raw candidate pool exceeds
// MaxSimilarityCandidates but the similarity threshold - not the cap - is
// what limits the result: if fewer of the capped candidates pass the
// threshold than the cap allowed, widening the cap wouldn't have surfaced
// more matches, so the "some candidates weren't considered" warning would be
// misleading.
func TestSearchSimilarToDoesNotReportCappedWhenThresholdIsTheLimit(t *testing.T) {
	docA := index.Document{
		ID:              "a.epub",
		Slug:            "doc-a",
		Metadata:        metadata.Metadata{Title: "Doc A", Authors: []string{"Author One"}, Format: "EPUB"},
		AuthorsSlugs:    []string{"author-one"},
		TextRankPhrases: []string{"oppenheimer manhattan", "project atomic", "los alamos"},
	}
	docStrong := index.Document{
		ID:              "strong.epub",
		Slug:            "strong-match",
		Metadata:        metadata.Metadata{Title: "Strong Match", Authors: []string{"Author Strong"}, Format: "EPUB"},
		AuthorsSlugs:    []string{"author-strong"},
		TextRankPhrases: []string{"oppenheimer manhattan", "project atomic", "los alamos"},
	}
	docs := []index.Document{docA, docStrong}
	for i := 1; i <= 3; i++ {
		docs = append(docs, index.Document{
			ID:              fmt.Sprintf("weak%d.epub", i),
			Slug:            fmt.Sprintf("weak-%d", i),
			Metadata:        metadata.Metadata{Title: fmt.Sprintf("Weak %d", i), Authors: []string{fmt.Sprintf("Author Weak %d", i)}, Format: "EPUB"},
			AuthorsSlugs:    []string{fmt.Sprintf("author-weak-%d", i)},
			TextRankPhrases: []string{"oppenheimer manhattan"},
		})
	}

	idx := newTextRankTestIndexWithConfig(t, docs, index.Config{
		MaxSimilarityCandidates: 2,
		MinSimilarityScoreRatio: 0.6,
	})

	res, err := idx.Search(index.SearchFields{SimilarTo: "doc-a"}, 1, 10)
	if err != nil {
		t.Fatalf("Search returned an error: %s", err)
	}

	if res.Candidates.Capped() {
		t.Errorf("expected Candidates.Capped to report false when the threshold, not the cap, limited the result (Total=%d, Cap=%d)", res.Candidates.Total(), res.Candidates.Cap())
	}
}

// TestSearchSimilarToRespectsSortBy guards against runSimilarityQuery
// ignoring SearchFields.SortBy: pruning weak matches needs the underlying
// Bleve query sorted by score (to find the best match's score), so the
// user's chosen display order can't just be handed to Bleve like
// runPaginatedQuery does - it has to be applied afterwards, over the
// already-pruned documents. All three documents here score similarly enough
// to survive pruning, so any difference in order is down to SortBy alone.
func TestSearchSimilarToRespectsSortBy(t *testing.T) {
	sharedPairs := []string{"oppenheimer manhattan", "project atomic", "los alamos"}

	docA := index.Document{
		ID:              "a.epub",
		Slug:            "doc-a",
		Metadata:        metadata.Metadata{Title: "Doc A", Authors: []string{"Author One"}, Format: "EPUB"},
		AuthorsSlugs:    []string{"author-one"},
		TextRankPhrases: sharedPairs,
	}
	docOldest := index.Document{
		ID:              "oldest.epub",
		Slug:            "oldest",
		Metadata:        metadata.Metadata{Title: "Oldest", Authors: []string{"Author Two"}, Format: "EPUB", Publication: precisiondate.NewPrecisionDate("2000-01-01T00:00:00Z", precisiondate.PrecisionDay)},
		AuthorsSlugs:    []string{"author-two"},
		TextRankPhrases: sharedPairs,
	}
	docMiddle := index.Document{
		ID:              "middle.epub",
		Slug:            "middle",
		Metadata:        metadata.Metadata{Title: "Middle", Authors: []string{"Author Three"}, Format: "EPUB", Publication: precisiondate.NewPrecisionDate("2010-01-01T00:00:00Z", precisiondate.PrecisionDay)},
		AuthorsSlugs:    []string{"author-three"},
		TextRankPhrases: sharedPairs,
	}
	docNewest := index.Document{
		ID:              "newest.epub",
		Slug:            "newest",
		Metadata:        metadata.Metadata{Title: "Newest", Authors: []string{"Author Four"}, Format: "EPUB", Publication: precisiondate.NewPrecisionDate("2020-01-01T00:00:00Z", precisiondate.PrecisionDay)},
		AuthorsSlugs:    []string{"author-four"},
		TextRankPhrases: sharedPairs,
	}

	idx := newTextRankTestIndex(t, []index.Document{docA, docOldest, docMiddle, docNewest})

	older, err := idx.Search(index.SearchFields{SimilarTo: "doc-a", SortBy: []string{"Publication.Date"}}, 1, 10)
	if err != nil {
		t.Fatalf("Search returned an error: %s", err)
	}
	wantOlderFirst := []string{"oldest", "middle", "newest"}
	if got := slugsOf(older.Hits()); !slices.Equal(got, wantOlderFirst) {
		t.Errorf("expected order (oldest first) %v, got %v", wantOlderFirst, got)
	}

	newer, err := idx.Search(index.SearchFields{SimilarTo: "doc-a", SortBy: []string{"-Publication.Date"}}, 1, 10)
	if err != nil {
		t.Fatalf("Search returned an error: %s", err)
	}
	wantNewerFirst := []string{"newest", "middle", "oldest"}
	if got := slugsOf(newer.Hits()); !slices.Equal(got, wantNewerFirst) {
		t.Errorf("expected order (newest first) %v, got %v", wantNewerFirst, got)
	}
}
