package index

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/svera/coreander/v5/internal/result"
)

func (b *BleveIndexer) searchSimilar(searchFields SearchFields, page, resultsPerPage int) (result.CappedPaginatedResult[[]Document], error) {
	filtersQuery := bleve.NewConjunctionQuery()
	doc, err := b.Document(searchFields.SimilarTo)
	if err != nil {
		return result.CappedPaginatedResult[[]Document]{}, err
	}
	stats, err := b.similarityStatistics()
	if err != nil {
		return result.CappedPaginatedResult[[]Document]{}, err
	}
	similarToQuery, err := b.similarToQuery(doc, stats)
	if err != nil {
		return result.CappedPaginatedResult[[]Document]{}, err
	}
	filtersQuery.AddQuery(similarToQuery)
	conjunctsBeforeFilters := len(filtersQuery.Conjuncts)
	b.addFilters(searchFields, filtersQuery)

	// Anchor the cutoff to the unfiltered best match only when filters differ.
	var scoringQuery query.Query
	if len(filtersQuery.Conjuncts) > conjunctsBeforeFilters {
		scoringQuery = similarToQuery
	}
	return b.runSimilarityQuery(scoringQuery, filtersQuery, page, resultsPerPage, searchFields.SortBy, float64(doc.Publication.Date))
}

// similarToQuery builds Search's SearchFields.SimilarTo candidate query:
// documents sharing the configured minimum of uncommon TextRank word pairs with doc, excluding doc itself,
// documents in the same series, and documents by the same authors. When both
// books have subjects known to be uncommon in the library, they must share
// at least one of those subjects, in addition to phrases.
// Missing or common-only subjects do not restrict phrase-qualified matches.
func (b *BleveIndexer) similarToQuery(doc Document, stats *similarityStatistics) (*query.BooleanQuery, error) {
	bq := bleve.NewBooleanQuery()

	// Array positions preserve phrase adjacency. A minimum number of distinct,
	// uncommon clauses must match before relative score pruning is considered.
	// Limit clauses only after filtering so generic phrases cannot crowd out
	// distinctive ones. Single TextRank words never qualify as evidence.
	phrases, err := b.distinctiveSimilarityPhrases(doc, stats)
	if err != nil {
		return nil, err
	}
	if len(phrases) < b.minSimilaritySharedPhrases {
		bq.AddMust(bleve.NewMatchNoneQuery())
		return bq, nil
	}
	phrasesCompoundQuery := bleve.NewDisjunctionQuery()
	phrasesCompoundQuery.SetMin(float64(b.minSimilaritySharedPhrases))
	for _, phrase := range phrases {
		kq := bleve.NewMatchPhraseQuery(phrase)
		kq.SetField("TextRankPhrases")
		kq.Analyzer = defaultAnalyzer
		phrasesCompoundQuery.AddQuery(kq)
	}
	bq.AddMust(phrasesCompoundQuery)

	addSimilaritySubjectFilter(doc, bq, stats)
	b.excludeSelfSeriesAndAuthors(doc, bq)

	return bq, nil
}

// excludeSelfSeriesAndAuthors adds the MustNot clauses used by
// similarToQuery: doc itself, any document in the same series, and any
// document by the same authors.
func (b *BleveIndexer) excludeSelfSeriesAndAuthors(doc Document, bq *query.BooleanQuery) {
	if doc.SeriesSlug != "" {
		sq := bleve.NewTermQuery(doc.SeriesSlug)
		sq.SetField("SeriesSlug")
		bq.AddMustNot(sq)
	}

	bq.AddMustNot(bleve.NewDocIDQuery([]string{doc.ID}))

	authorsCompoundQuery := bleve.NewDisjunctionQuery()
	for _, slug := range doc.AuthorsSlugs {
		qa := bleve.NewTermQuery(slug)
		qa.SetField("AuthorsSlugs")
		authorsCompoundQuery.AddQuery(qa)
	}
	bq.AddMustNot(authorsCompoundQuery)
}

func (b *BleveIndexer) distinctiveSimilarityPhrases(doc Document, stats *similarityStatistics) ([]string, error) {
	if stats == nil {
		return nil, nil
	}
	b.documentsMu.RLock()
	analyzer := b.documentsIdx.Mapping().AnalyzerNamed(defaultAnalyzer)
	b.documentsMu.RUnlock()
	if analyzer == nil {
		return nil, fmt.Errorf("missing similarity phrase analyzer %q", defaultAnalyzer)
	}
	threshold := similarityPhraseThreshold(stats.DocumentCount, b.maxSimilarityPhraseFrequency)
	phrases := make([]string, 0, len(doc.TextRankPhrases))
	seen := make(map[string]bool)
	for _, phrase := range doc.TextRankPhrases {
		key := similarityPhraseKey(phrase, analyzer)
		count, known := stats.PhraseCounts[key]
		if key == "" || seen[key] || !known || count < similarityMinPhraseDocumentFrequency || float64(count) > threshold {
			continue
		}
		seen[key] = true
		phrases = append(phrases, phrase)
		if b.maxSimilarityPhrases > 0 && len(phrases) == b.maxSimilarityPhrases {
			break
		}
	}
	return phrases, nil
}

func uncommonSimilaritySubjects(subjects, knownUncommonSubjects []string) []string {
	var uncommon []string
	for _, subject := range subjects {
		_, knownUncommon := slices.BinarySearch(knownUncommonSubjects, subject)
		if knownUncommon && !slices.Contains(uncommon, subject) {
			uncommon = append(uncommon, subject)
		}
	}
	return uncommon
}

func uncommonSimilaritySubjectsQuery(subjects []string) query.Query {
	uncommon := bleve.NewDisjunctionQuery()
	for _, subject := range subjects {
		q := bleve.NewTermQuery(subject)
		q.SetField("SubjectsSlugs")
		uncommon.AddQuery(q)
	}
	return uncommon
}

func addSimilaritySubjectFilter(reference Document, candidates *query.BooleanQuery, stats *similarityStatistics) {
	subjects := uncommonSimilaritySubjects(reference.SubjectsSlugs, stats.uncommonSubjects)
	if len(subjects) == 0 {
		return
	}
	shared := uncommonSimilaritySubjectsQuery(subjects)
	noUncommonSubjects := bleve.NewBooleanQuery()
	noUncommonSubjects.AddMust(bleve.NewMatchAllQuery())
	noUncommonSubjects.AddMustNot(stats.subjectQuery)
	candidates.AddFilter(bleve.NewDisjunctionQuery(shared, noUncommonSubjects))
}

// runSimilarityQuery caps candidates by score, applies the relative cutoff,
// then sorts and paginates survivors. An optional unfiltered scoringQuery keeps
// user filters from lowering the cutoff. referenceDate breaks display-sort ties.
func (b *BleveIndexer) runSimilarityQuery(scoringQuery, candidateQuery query.Query, page, resultsPerPage int, sortBy []string, referenceDate float64) (result.CappedPaginatedResult[[]Document], error) {
	// sortBy is deliberately not passed to searchAndHydrate: pruning below needs the
	// underlying Bleve query sorted by score.
	searchResult, hydratedDocs, err := b.searchAndHydrate(candidateQuery, b.maxSimilarityCandidates, 0, nil)
	if err != nil {
		return result.CappedPaginatedResult[[]Document]{}, err
	}

	if searchResult.Total == 0 || len(searchResult.Hits) == 0 {
		return result.CappedPaginatedResult[[]Document]{}, nil
	}

	bestScore := searchResult.Hits[0].Score
	if scoringQuery != nil {
		bestScore, err = b.bestScore(scoringQuery)
		if err != nil {
			return result.CappedPaginatedResult[[]Document]{}, err
		}
		if bestScore == 0 {
			return result.CappedPaginatedResult[[]Document]{}, nil
		}
	}

	threshold := bestScore * b.minSimilarityScoreRatio

	hits := make([]similarityHit, 0, len(hydratedDocs))
	for i, hit := range searchResult.Hits {
		if hit.Score < threshold {
			continue
		}
		hits = append(hits, similarityHit{doc: hydratedDocs[i], score: hit.Score})
	}

	sortSimilarityResults(hits, sortBy, referenceDate)

	docs := make([]Document, len(hits))
	for i, h := range hits {
		docs[i] = h.doc
	}

	paginated := result.Paginate(resultsPerPage, page, len(docs), docs)
	return result.CappedPaginatedResult[[]Document]{
		Paginated:  paginated,
		Candidates: result.NewCandidatePool(int(searchResult.Total), len(hits), b.maxSimilarityCandidates),
	}, nil
}

// bestScore returns the top score query would produce, or 0 if it has no matches.
func (b *BleveIndexer) bestScore(query query.Query) (float64, error) {
	searchOptions := bleve.NewSearchRequestOptions(query, 1, 0, false)
	b.documentsMu.RLock()
	searchResult, err := b.documentsIdx.Search(searchOptions)
	b.documentsMu.RUnlock()
	if err != nil {
		return 0, err
	}
	if len(searchResult.Hits) == 0 {
		return 0, nil
	}
	return searchResult.Hits[0].Score, nil
}

type similarityHit struct {
	doc   Document
	score float64
}

// Preserve score order for unresolved ties, preferring nearby publication dates
// when the reference date is known.
func sortSimilarityResults(hits []similarityHit, sortBy []string, referenceDate float64) {
	if len(sortBy) == 0 && referenceDate == 0 {
		return
	}
	sort.SliceStable(hits, func(i, j int) bool {
		for _, key := range sortBy {
			if strings.TrimPrefix(key, "-") == "_score" {
				if hits[i].score == hits[j].score {
					continue
				}
				if strings.HasPrefix(key, "-") {
					return hits[i].score > hits[j].score
				}
				return hits[i].score < hits[j].score
			}
			if c := compareDocumentsBySortKey(hits[i].doc, hits[j].doc, key); c != 0 {
				return c < 0
			}
		}
		if referenceDate == 0 {
			return false
		}
		di := math.Abs(float64(hits[i].doc.Publication.Date) - referenceDate)
		dj := math.Abs(float64(hits[j].doc.Publication.Date) - referenceDate)
		return di < dj
	})
}

// compareDocumentsBySortKey compares a and b by key, one of the field names (an
// optional "-" prefix reverses the comparison) parseDocumentSortBy produces.
// Returns <0, 0 or >0, like strings.Compare/cmp.Compare.
func compareDocumentsBySortKey(a, b Document, key string) int {
	desc := strings.HasPrefix(key, "-")
	field := strings.TrimPrefix(key, "-")

	var c int
	switch field {
	case "Publication.Date":
		c = cmp.Compare(a.Publication.Date, b.Publication.Date)
	case "Words":
		c = cmp.Compare(a.Words, b.Words)
	case "DataSourceScoreValue":
		if (a.DataSourceScoreValue == 0) != (b.DataSourceScoreValue == 0) {
			if a.DataSourceScoreValue == 0 {
				return 1
			}
			return -1
		}
		c = cmp.Compare(a.DataSourceScoreValue, b.DataSourceScoreValue)
	case "Series":
		c = cmp.Compare(a.Series, b.Series)
	case "SeriesIndex":
		c = cmp.Compare(a.SeriesIndex, b.SeriesIndex)
	}
	if desc {
		c = -c
	}
	return c
}
