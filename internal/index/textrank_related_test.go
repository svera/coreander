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

func TestSameSubjectsMatchesByTextRankKeywords(t *testing.T) {
	sharedPairs := []string{"oppenheimer manhattan", "atomic bomb", "los alamos"}

	docWithSubject := index.Document{
		ID:            "with-subject.epub",
		Slug:          "with-subject",
		Metadata:      metadata.Metadata{Title: "With Subject", Authors: []string{"Author One"}, Format: "EPUB", Subjects: []string{"History"}},
		AuthorsSlugs:  []string{"author-one"},
		SubjectsSlugs: []string{"history"},
		// docWithSubject also carries the same TextRankKeywords pairs as
		// docSharedKeywordsOnly, so it is expected to match both via subject
		// and via keywords.
		TextRankKeywords: sharedPairs,
	}
	docSharedSubject := index.Document{
		ID:            "shared-subject.epub",
		Slug:          "shared-subject",
		Metadata:      metadata.Metadata{Title: "Shared Subject", Authors: []string{"Author Two"}, Format: "EPUB", Subjects: []string{"History"}},
		AuthorsSlugs:  []string{"author-two"},
		SubjectsSlugs: []string{"history"},
		// No TextRankKeywords: this document should still be found by a
		// formal subject match, same as before TextRank keywords existed.
	}
	docSharedKeywordsOnly := index.Document{
		ID:               "shared-keywords.epub",
		Slug:             "shared-keywords",
		Metadata:         metadata.Metadata{Title: "Shared Keywords", Authors: []string{"Author Three"}, Format: "EPUB"},
		AuthorsSlugs:     []string{"author-three"},
		TextRankKeywords: sharedPairs,
	}
	docUnrelated := index.Document{
		ID:               "unrelated.epub",
		Slug:             "unrelated",
		Metadata:         metadata.Metadata{Title: "Unrelated", Authors: []string{"Author Four"}, Format: "EPUB"},
		AuthorsSlugs:     []string{"author-four"},
		TextRankKeywords: []string{"cooking recipes", "bread baking", "gardening tools"},
	}
	docSameAuthor := index.Document{
		ID:               "same-author.epub",
		Slug:             "same-author",
		Metadata:         metadata.Metadata{Title: "Same Author", Authors: []string{"Author One"}, Format: "EPUB"},
		AuthorsSlugs:     []string{"author-one"},
		TextRankKeywords: sharedPairs,
	}

	idx := newTextRankTestIndex(t, []index.Document{
		docWithSubject, docSharedSubject, docSharedKeywordsOnly, docUnrelated, docSameAuthor,
	})

	got, err := idx.SameSubjects("with-subject", 10)
	if err != nil {
		t.Fatalf("SameSubjects returned an error: %s", err)
	}

	gotSlugs := slugsOf(got)
	wantSlugs := []string{"shared-subject", "shared-keywords"}
	for _, want := range wantSlugs {
		if !slices.Contains(gotSlugs, want) {
			t.Errorf("expected %q to be present in results %v", want, gotSlugs)
		}
	}
	if slices.Contains(gotSlugs, "unrelated") {
		t.Errorf("did not expect unrelated document to match, got %v", gotSlugs)
	}
	if slices.Contains(gotSlugs, "same-author") {
		t.Errorf("did not expect same-author document to match (SameSubjects excludes same-author documents), got %v", gotSlugs)
	}
}

func TestSearchSimilarToMatchesSameSubjectsResults(t *testing.T) {
	docA := index.Document{
		ID:               "a.epub",
		Slug:             "doc-a",
		Metadata:         metadata.Metadata{Title: "Doc A", Authors: []string{"Author One"}, Format: "EPUB", Subjects: []string{"History", "Physics"}},
		AuthorsSlugs:     []string{"author-one"},
		SubjectsSlugs:    []string{"history", "physics"},
		TextRankKeywords: []string{"oppenheimer manhattan", "project atomic", "los alamos", "physics nuclear"},
	}
	docSameSubject := index.Document{
		ID:            "same-subject.epub",
		Slug:          "same-subject",
		Metadata:      metadata.Metadata{Title: "Same Subject", Authors: []string{"Author Two"}, Format: "EPUB", Subjects: []string{"History", "Physics"}},
		AuthorsSlugs:  []string{"author-two"},
		SubjectsSlugs: []string{"history", "physics"},
	}
	docSharedKeywords := index.Document{
		ID:               "shared-keywords.epub",
		Slug:             "shared-keywords",
		Metadata:         metadata.Metadata{Title: "Shared Keywords", Authors: []string{"Author Three"}, Format: "EPUB"},
		AuthorsSlugs:     []string{"author-three"},
		TextRankKeywords: []string{"oppenheimer manhattan", "project atomic", "los alamos"},
	}
	docUnrelated := index.Document{
		ID:               "unrelated.epub",
		Slug:             "unrelated",
		Metadata:         metadata.Metadata{Title: "Unrelated", Authors: []string{"Author Four"}, Format: "EPUB"},
		AuthorsSlugs:     []string{"author-four"},
		TextRankKeywords: []string{"cooking recipes", "bread baking", "gardening tools"},
	}

	idx := newTextRankTestIndex(t, []index.Document{docA, docSameSubject, docSharedKeywords, docUnrelated})

	widgetResults, err := idx.SameSubjects("doc-a", 4)
	if err != nil {
		t.Fatalf("SameSubjects returned an error: %s", err)
	}

	seeAllResults, err := idx.Search(index.SearchFields{SimilarTo: "doc-a"}, 1, 10)
	if err != nil {
		t.Fatalf("Search returned an error: %s", err)
	}

	widgetSlugs := slugsOf(widgetResults)
	seeAllSlugs := slugsOf(seeAllResults.Hits())

	slices.Sort(widgetSlugs)
	slices.Sort(seeAllSlugs)

	if !slices.Equal(widgetSlugs, seeAllSlugs) {
		t.Errorf("expected the \"See all\" search to return the same documents as the SameSubjects widget: widget=%v, search=%v", widgetSlugs, seeAllSlugs)
	}
	if slices.Contains(seeAllSlugs, "unrelated") {
		t.Errorf("did not expect unrelated document to match, got %v", seeAllSlugs)
	}
}

func TestSearchSimilarToPrunesWeakMatches(t *testing.T) {
	docA := index.Document{
		ID:               "a.epub",
		Slug:             "doc-a",
		Metadata:         metadata.Metadata{Title: "Doc A", Authors: []string{"Author One"}, Format: "EPUB"},
		AuthorsSlugs:     []string{"author-one"},
		TextRankKeywords: []string{"oppenheimer manhattan", "project atomic", "los alamos", "physics nuclear"},
	}
	docStrongMatch := index.Document{
		ID:               "strong.epub",
		Slug:             "strong-match",
		Metadata:         metadata.Metadata{Title: "Strong Match", Authors: []string{"Author Two"}, Format: "EPUB"},
		AuthorsSlugs:     []string{"author-two"},
		TextRankKeywords: []string{"oppenheimer manhattan", "project atomic", "los alamos"},
	}
	docWeakMatch := index.Document{
		ID:           "weak.epub",
		Slug:         "weak-match",
		Metadata:     metadata.Metadata{Title: "Weak Match", Authors: []string{"Author Three"}, Format: "EPUB"},
		AuthorsSlugs: []string{"author-three"},
		// Shares only one pair ("physics nuclear") with doc-a - should score
		// far below the strong match and get pruned by the similarity
		// threshold.
		TextRankKeywords: []string{"physics nuclear", "cooking recipes", "bread baking", "gardening tools"},
	}
	docUnrelated := index.Document{
		ID:               "unrelated.epub",
		Slug:             "unrelated",
		Metadata:         metadata.Metadata{Title: "Unrelated", Authors: []string{"Author Four"}, Format: "EPUB"},
		AuthorsSlugs:     []string{"author-four"},
		TextRankKeywords: []string{"completely unrelated", "cooking recipes", "bread baking"},
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
// resulting Paginated reports the cap was hit (CandidatesCapped) along with
// the true, uncapped match count (CandidatesTotal) - so a caller (e.g. the
// search UI's "similar to" banner) can tell the user some matches weren't
// even considered, rather than silently dropping them.
func TestSearchSimilarToReportsCappedCandidates(t *testing.T) {
	sharedKeywords := []string{"oppenheimer manhattan", "project atomic", "los alamos"}

	docA := index.Document{
		ID:               "a.epub",
		Slug:             "doc-a",
		Metadata:         metadata.Metadata{Title: "Doc A", Authors: []string{"Author One"}, Format: "EPUB"},
		AuthorsSlugs:     []string{"author-one"},
		TextRankKeywords: sharedKeywords,
	}
	docs := []index.Document{docA}
	for i := 1; i <= 4; i++ {
		docs = append(docs, index.Document{
			ID:               fmt.Sprintf("match%d.epub", i),
			Slug:             fmt.Sprintf("match-%d", i),
			Metadata:         metadata.Metadata{Title: fmt.Sprintf("Match %d", i), Authors: []string{fmt.Sprintf("Author %d", i)}, Format: "EPUB"},
			AuthorsSlugs:     []string{fmt.Sprintf("author-%d", i)},
			TextRankKeywords: sharedKeywords,
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

	if !res.CandidatesCapped() {
		t.Errorf("expected CandidatesCapped to report true with 4 matches and a cap of 2, got false (CandidatesTotal=%d, CandidatesCap=%d)", res.CandidatesTotal(), res.CandidatesCap())
	}
	if res.CandidatesTotal() != 4 {
		t.Errorf("expected CandidatesTotal to report the uncapped match count 4, got %d", res.CandidatesTotal())
	}
	if res.CandidatesCap() != 2 {
		t.Errorf("expected CandidatesCap to report the configured cap 2, got %d", res.CandidatesCap())
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
		ID:               "a.epub",
		Slug:             "doc-a",
		Metadata:         metadata.Metadata{Title: "Doc A", Authors: []string{"Author One"}, Format: "EPUB"},
		AuthorsSlugs:     []string{"author-one"},
		TextRankKeywords: sharedPairs,
	}
	docOldest := index.Document{
		ID:               "oldest.epub",
		Slug:             "oldest",
		Metadata:         metadata.Metadata{Title: "Oldest", Authors: []string{"Author Two"}, Format: "EPUB", Publication: precisiondate.NewPrecisionDate("2000-01-01T00:00:00Z", precisiondate.PrecisionDay)},
		AuthorsSlugs:     []string{"author-two"},
		TextRankKeywords: sharedPairs,
	}
	docMiddle := index.Document{
		ID:               "middle.epub",
		Slug:             "middle",
		Metadata:         metadata.Metadata{Title: "Middle", Authors: []string{"Author Three"}, Format: "EPUB", Publication: precisiondate.NewPrecisionDate("2010-01-01T00:00:00Z", precisiondate.PrecisionDay)},
		AuthorsSlugs:     []string{"author-three"},
		TextRankKeywords: sharedPairs,
	}
	docNewest := index.Document{
		ID:               "newest.epub",
		Slug:             "newest",
		Metadata:         metadata.Metadata{Title: "Newest", Authors: []string{"Author Four"}, Format: "EPUB", Publication: precisiondate.NewPrecisionDate("2020-01-01T00:00:00Z", precisiondate.PrecisionDay)},
		AuthorsSlugs:     []string{"author-four"},
		TextRankKeywords: sharedPairs,
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
