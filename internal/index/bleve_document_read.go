package index

import (
	"cmp"
	"errors"
	"html/template"
	"image"
	"math"
	"net/url"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/gosimple/slug"
	"github.com/rickb777/date/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v5/internal/metadata"
	"github.com/svera/coreander/v5/internal/precisiondate"
	"github.com/svera/coreander/v5/internal/result"
)

// titleBoost multiplies the score contribution of a Title match relative to a Series match, so a
// document whose own title matches ranks above other entries in the same series that only match
// because they share its series name.
const titleBoost = 3.0

// DefaultDocumentSortBy is the "relevance" sort order applied whenever no explicit
// sort is requested: highest score first, then by series/series index for
// documents that tie on score (e.g. several entries of the same series matching
// a keyword search equally). SearchFields.SimilarTo callers and SameSubjects both
// use it, so a document appears in the same relative order in search-similar
// results and in the "related documents" section.
var DefaultDocumentSortBy = []string{"-_score", "Series", "SeriesIndex"}

// Search look for documents which match the passed keywords and filters.
// Returns a maximum <resultsPerPage> documents, offset by <page>
func (b *BleveIndexer) Search(searchFields SearchFields, page, resultsPerPage int) (result.Paginated[[]Document], error) {
	filtersQuery := bleve.NewConjunctionQuery()

	if searchFields.SimilarTo != "" {
		doc, err := b.Document(searchFields.SimilarTo)
		if err != nil {
			return result.Paginated[[]Document]{}, err
		}
		subjectsQuery := b.subjectsQuery(doc)
		filtersQuery.AddQuery(subjectsQuery)
		conjunctsBeforeFilters := len(filtersQuery.Conjuncts)
		b.addFilters(searchFields, filtersQuery)

		// Only score subjectsQuery on its own when addFilters actually added
		// something to filtersQuery: otherwise the two queries are equivalent, and
		// scoring subjectsQuery separately would just make Bleve evaluate the same
		// matches twice for no benefit - see runSimilarityQuery.
		var scoringQuery query.Query
		if len(filtersQuery.Conjuncts) > conjunctsBeforeFilters {
			scoringQuery = subjectsQuery
		}
		return b.runSimilarityQuery(scoringQuery, filtersQuery, page, resultsPerPage, searchFields.SortBy, float64(doc.Publication.Date))
	}

	if searchFields.Keywords != "" {
		for _, prefix := range []string{"Authors:", "Illustrators:", "Series:", "Title:", "Subjects:", "\""} {
			if strings.HasPrefix(strings.Trim(searchFields.Keywords, " "), prefix) {
				query := bleve.NewQueryStringQuery(searchFields.Keywords)
				filtersQuery.AddQuery(query)
				b.addFilters(searchFields, filtersQuery)

				return b.runPaginatedQuery(filtersQuery, page, resultsPerPage, searchFields.SortBy)
			}
		}

		for _, prefix := range []string{"AuthorsSlugs:", "IllustratorsSlugs:", "SeriesSlug:"} {
			unescaped, err := url.QueryUnescape(strings.TrimSpace(searchFields.Keywords))
			if err != nil {
				break
			}
			if !strings.HasPrefix(unescaped, prefix) {
				continue
			}
			unescaped = strings.TrimPrefix(unescaped, prefix)
			terms := strings.Split(unescaped, ",")
			qb := bleve.NewDisjunctionQuery()
			for _, term := range terms {
				qs := bleve.NewTermQuery(term)
				qs.SetField(strings.TrimSuffix(prefix, ":"))
				qb.AddQuery(qs)
			}
			filtersQuery.AddQuery(qb)
			b.addFilters(searchFields, filtersQuery)
			return b.runPaginatedQuery(filtersQuery, page, resultsPerPage, searchFields.SortBy)
		}

		analyzers, err := b.analyzers()
		if err != nil {
			return result.Paginated[[]Document]{}, err
		}

		query := b.composeQuery(searchFields.Keywords, analyzers)
		filtersQuery.AddQuery(query)
	} else {
		// When no keywords are provided, use MatchAllQuery to return all documents
		// Filters will still be applied on top of this
		matchAllQuery := bleve.NewMatchAllQuery()
		filtersQuery.AddQuery(matchAllQuery)
	}

	b.addFilters(searchFields, filtersQuery)

	return b.runPaginatedQuery(filtersQuery, page, resultsPerPage, searchFields.SortBy)
}

// newInclusiveNumericRangeQuery builds a numeric range query inclusive on both ends.
// bleve.NewNumericRangeQuery defaults to an exclusive upper bound ([min, max)), which would
// silently drop documents whose value exactly equals the "to" boundary of a range filter.
func newInclusiveNumericRangeQuery(min, max *float64) *query.NumericRangeQuery {
	inclusive := true
	return bleve.NewNumericRangeInclusiveQuery(min, max, &inclusive, &inclusive)
}

// addFilters narrows filtersQuery to searchFields' constraints (language, subjects,
// date/reading-time/pages ranges, illustrated-only). Every query added here has its
// boost explicitly zeroed: filtersQuery is a ConjunctionQuery, which sums the scores of
// all its matching clauses, so an unboosted filter would otherwise inject its own
// relevance score (e.g. a PrefixQuery's IDF-based score, which varies per document
// depending on how many other documents share its exact field value) into the result
// ranking, on top of whatever relevance query (keyword search, TextRank similarity...)
// filtersQuery is also being used for. A filter should only ever narrow which documents
// match, never influence how they're ordered.
func (b *BleveIndexer) addFilters(searchFields SearchFields, filtersQuery *query.ConjunctionQuery) {
	// Only filter by language if a language is specified
	if searchFields.Language != "" && strings.TrimSpace(searchFields.Language) != "" {
		// Use prefix query to match all regional variants of the selected language
		// e.g., selecting "es" will match "es", "es_MX", "es-ES", "es-CL", etc.
		q := bleve.NewPrefixQuery(strings.TrimSpace(searchFields.Language))
		q.SetField("Language")
		q.SetBoost(0)
		filtersQuery.AddQuery(q)
	}
	// Only filter by subject if a subject is specified
	if searchFields.Subjects != "" && strings.TrimSpace(searchFields.Subjects) != "" {
		// Support multiple subjects (comma-separated) - using AND logic
		subjectStrings := strings.Split(searchFields.Subjects, ",")
		subjectQueries := bleve.NewConjunctionQuery()

		for _, subjectStr := range subjectStrings {
			subjectStr = strings.TrimSpace(subjectStr)
			if subjectStr == "" {
				continue
			}
			// Convert subject to slug for exact matching on SubjectsSlugs (same as Subjects() and indexing)
			subjectSlug := slug.Make(subjectStr)
			// Use TermQuery for exact match on SubjectsSlugs (keyword field, not analyzed)
			q := bleve.NewTermQuery(subjectSlug)
			q.SetField("SubjectsSlugs")
			q.SetBoost(0)
			subjectQueries.AddQuery(q)
		}

		// Only add query if we have at least one valid subject
		if len(subjectQueries.Conjuncts) > 0 {
			subjectQueries.SetBoost(0)
			filtersQuery.AddQuery(subjectQueries)
		}
	}
	addDateRangeFilter(filtersQuery, "Publication.Date", searchFields.PubDateFrom, searchFields.PubDateTo)
	if searchFields.EstReadTimeFrom > 0 || searchFields.EstReadTimeTo > 0 {
		var min, max *float64
		if searchFields.EstReadTimeFrom > 0 {
			minVal := searchFields.EstReadTimeFrom * 60 * searchFields.WordsPerMinute
			min = &minVal
		}
		if searchFields.EstReadTimeTo > 0 {
			maxVal := searchFields.EstReadTimeTo * 60 * searchFields.WordsPerMinute
			max = &maxVal
		}
		wordsQuery := newInclusiveNumericRangeQuery(min, max)
		wordsQuery.SetField("Words")
		wordsQuery.SetBoost(0)

		// PDF documents are always indexed with Words == 0, so a "Words in [x,y]" range with no
		// lower bound would otherwise also match every PDF, as if it had zero reading time.
		// Exclude PDFs explicitly rather than requiring Words > 0: some EPUBs are also indexed
		// with Words == 0 when word-count extraction failed for them, and should still be
		// findable rather than being silently hidden by every reading time filter.
		excludePDF := bleve.NewTermQuery("pdf")
		excludePDF.SetField("Format")

		bq := bleve.NewBooleanQuery()
		bq.AddMust(wordsQuery)
		bq.AddMustNot(excludePDF)
		bq.SetBoost(0)
		filtersQuery.AddQuery(bq)
	}
	if searchFields.PagesFrom > 0 || searchFields.PagesTo > 0 {
		var min, max *float64
		if searchFields.PagesFrom > 0 {
			min = &searchFields.PagesFrom
		}
		if searchFields.PagesTo > 0 {
			max = &searchFields.PagesTo
		}
		pagesQuery := newInclusiveNumericRangeQuery(min, max)
		pagesQuery.SetField("Pages")
		pagesQuery.SetBoost(0)

		// EPUB documents are always indexed with Pages == 0, so a "Pages in [x,y]" range with no
		// lower bound would otherwise also match every EPUB, as if it had zero pages. Exclude
		// EPUBs explicitly rather than requiring Pages > 0, for the same reason as above.
		excludeEPUB := bleve.NewTermQuery("epub")
		excludeEPUB.SetField("Format")

		bq := bleve.NewBooleanQuery()
		bq.AddMust(pagesQuery)
		bq.AddMustNot(excludeEPUB)
		bq.SetBoost(0)
		filtersQuery.AddQuery(bq)
	}
	if searchFields.IllustratedOnly && b.illustratedMinAmount > 0 {
		minIllustrations := float64(b.illustratedMinAmount)
		q := bleve.NewNumericRangeQuery(&minIllustrations, nil)
		q.SetField("Illustrations")
		q.SetBoost(0)
		filtersQuery.AddQuery(q)
	}
}

func (b *BleveIndexer) composeQuery(keywords string, analyzers []string) *query.DisjunctionQuery {
	langCompoundQuery := bleve.NewDisjunctionQuery()
	// Special query for searches using partial title names and author names
	authorTitleQuery := bleve.NewConjunctionQuery()
	allLangsOrTitleQuery := bleve.NewDisjunctionQuery()

	// meaningfulKeywords drops words that are stop words (e.g. Spanish "de", "el", "los") in any
	// of the library's active languages. It's used only for the permissive OR-fallback queries
	// below (orTitleQuery's defaultAnalyzer case and orAuthorQuery), which otherwise treat any
	// single matching word, however common, as a strong match anchor across every document
	// containing it. The stricter AND-based queries above keep the original keywords, since for
	// those a literal connector word can be a meaningful part of an exact title/series match.
	meaningfulKeywords := b.stripCommonWords(keywords, analyzers)

	for _, analyzer := range analyzers {
		noStopWordsAnalyzer := analyzer
		if analyzer != defaultAnalyzer && analyzer != "" {
			noStopWordsAnalyzer = analyzer + "_no_stop_words"
		}

		qt := bleve.NewMatchQuery(keywords)
		qt.Analyzer = noStopWordsAnalyzer
		qt.SetField("Title")
		qt.Operator = query.MatchQueryOperatorAnd
		// A document's own title matching the query is a stronger relevance signal than the
		// query matching the name of the series it belongs to (which every entry in that series
		// shares, whether or not it's the specific entry being searched for).
		qt.SetBoost(titleBoost)

		qs := bleve.NewMatchQuery(keywords)
		qs.Analyzer = noStopWordsAnalyzer
		qs.SetField("Series")
		qs.Operator = query.MatchQueryOperatorAnd

		qd := bleve.NewMatchQuery(keywords)
		qd.Analyzer = analyzer
		qd.SetField("Description")
		qd.Operator = query.MatchQueryOperatorAnd

		qtr := bleve.NewMatchQuery(keywords)
		qtr.Analyzer = analyzer
		qtr.SetField("TextRankKeywords")
		qtr.Operator = query.MatchQueryOperatorAnd

		langCompoundQuery.AddQuery(qt, qs, qd, qtr)

		orTitleQuery := bleve.NewMatchQuery(keywords)
		orTitleQuery.SetField("Title")
		orTitleQuery.Operator = query.MatchQueryOperatorOr
		orTitleQuery.Analyzer = analyzer
		if analyzer == defaultAnalyzer {
			// default_analyzer has no stop-word filter of its own, so fall back to the
			// cross-language filtered keywords to avoid matching on bare connector words.
			orTitleQuery.Match = meaningfulKeywords
		}

		allLangsOrTitleQuery.AddQuery(orTitleQuery)
	}

	qa := bleve.NewMatchQuery(keywords)
	qa.SetField("Authors")
	qa.Operator = query.MatchQueryOperatorAnd
	qa.Analyzer = defaultAnalyzer

	qi := bleve.NewMatchQuery(keywords)
	qi.SetField("Illustrators")
	qi.Operator = query.MatchQueryOperatorAnd
	qi.Analyzer = defaultAnalyzer

	// Authors is always analyzed with defaultAnalyzer (it isn't language-specific), which has no
	// stop-word filter, so it uses meaningfulKeywords for the same reason as orTitleQuery above.
	orAuthorQuery := bleve.NewMatchQuery(meaningfulKeywords)
	orAuthorQuery.SetField("Authors")
	orAuthorQuery.Operator = query.MatchQueryOperatorOr
	orAuthorQuery.Analyzer = defaultAnalyzer

	authorTitleQuery.AddQuery(orAuthorQuery, allLangsOrTitleQuery)

	return bleve.NewDisjunctionQuery(qa, qi, langCompoundQuery, authorTitleQuery)
}

// stripCommonWords removes words from keywords that are treated as stop words by any of the
// library's active (real, stop-word-aware) language analyzers. A word only survives if at least
// one active language's analyzer keeps it, so genuinely meaningful words (author names, rare
// terms) are preserved even if they happen to be a stop word in an unrelated language.
func (b *BleveIndexer) stripCommonWords(keywords string, analyzers []string) string {
	words := strings.Fields(keywords)
	if len(words) <= 1 {
		return keywords
	}

	b.documentsMu.RLock()
	mapping := b.documentsIdx.Mapping()
	b.documentsMu.RUnlock()
	kept := make([]string, 0, len(words))
	for _, word := range words {
		isStopWord := false
		for _, analyzerName := range analyzers {
			if analyzerName == defaultAnalyzer {
				continue
			}
			analyzer := mapping.AnalyzerNamed(analyzerName)
			if analyzer == nil {
				continue
			}
			if len(analyzer.Analyze([]byte(word))) == 0 {
				isStopWord = true
				break
			}
		}
		if !isStopWord {
			kept = append(kept, word)
		}
	}

	if len(kept) == 0 {
		return keywords
	}
	return strings.Join(kept, " ")
}

func (b *BleveIndexer) runQuery(query query.Query, results int, sortBy []string) ([]Document, error) {
	res, err := b.runPaginatedQuery(query, 0, results, sortBy)
	if err != nil {
		return nil, err
	}
	return res.Hits(), nil
}

// searchAndHydrate runs a Bleve search for query (size hits starting at offset,
// fetching every stored field), sorted by sortBy if non-empty or else Bleve's
// default relevance order, and hydrates every hit into a Document. Shared by
// runPaginatedQuery and runSimilarityQuery, which differ only in how they turn
// the resulting hits into a paginated result.
func (b *BleveIndexer) searchAndHydrate(query query.Query, size, offset int, sortBy []string) (*bleve.SearchResult, []Document, error) {
	searchOptions := bleve.NewSearchRequestOptions(query, size, offset, false)
	// NewSearchRequestOptions defaults to sorting by relevance (-_score). Only override it when
	// the caller actually asked for a specific order: SortBy([]string{}) replaces that default
	// with an empty sort order, which falls back to arbitrary (index) order instead of relevance.
	if len(sortBy) > 0 {
		searchOptions.SortBy(sortBy)
	}
	searchOptions.Fields = []string{"*"}
	b.documentsMu.RLock()
	searchResult, err := b.documentsIdx.Search(searchOptions)
	b.documentsMu.RUnlock()
	if err != nil {
		return nil, nil, err
	}

	docs := make([]Document, len(searchResult.Hits))
	for i, hit := range searchResult.Hits {
		docs[i] = hydrateDocument(hit)
	}

	return searchResult, docs, nil
}

func (b *BleveIndexer) runPaginatedQuery(query query.Query, page, resultsPerPage int, sortBy []string) (result.Paginated[[]Document], error) {
	if page < 1 {
		page = 1
	}

	searchResult, docs, err := b.searchAndHydrate(query, resultsPerPage, (page-1)*resultsPerPage, sortBy)
	if err != nil {
		return result.Paginated[[]Document]{}, err
	}

	if searchResult.Total == 0 {
		return result.Paginated[[]Document]{}, nil
	}

	return result.NewPaginated(
		resultsPerPage,
		page,
		int(searchResult.Total),
		docs,
	), nil
}

// runSimilarityQuery is like runPaginatedQuery, but for score-based "similar document"
// queries (see SearchFields.SimilarTo): relying on Bleve's own offset/limit pagination
// isn't enough here, since a document weakly matching on only one shared term would
// otherwise take up a page slot next to strongly related ones. Instead this fetches up
// to b.maxSimilarityCandidates top-scoring matches from candidateQuery, drops any
// scoring below b.minSimilarityScoreRatio of the best match, and paginates over what's
// left.
//
// The best match's score is normally just candidateQuery's own top hit. But when
// scoringQuery is non-nil (candidateQuery is subjectsQuery plus extra filters -
// language, publication date range, etc. - that scoringQuery, subjectsQuery alone,
// doesn't have) the best score is taken from scoringQuery instead, at the cost of
// evaluating that query too: those filters can shrink the candidate pool without the
// reference document actually having fewer or weaker true matches, and if the
// threshold were based on candidateQuery's own best score, filtering to a language
// with only a weak match would lower the bar and let other weak, effectively
// unrelated matches in that language through. The ratio should stay anchored to how
// well the best actually-similar document (in any language) matches, not shift
// depending on which of those matches survive the filters.
//
// Pruning always needs the underlying Bleve query sorted by score, to find the best
// match's score - so sortBy (the user's chosen display order, e.g. by publication date)
// can't be handed to Bleve the way runPaginatedQuery does. Instead it's applied
// afterwards, in Go, over the already-pruned documents; see sortSimilarityResults.
// referenceDate, if non-zero, is the publication date of the document these results are
// similar to, used as a tiebreaker (closest first) between equally-ranked documents.
func (b *BleveIndexer) runSimilarityQuery(scoringQuery, candidateQuery query.Query, page, resultsPerPage int, sortBy []string, referenceDate float64) (result.Paginated[[]Document], error) {
	// sortBy is deliberately not passed to searchAndHydrate: pruning below needs the
	// underlying Bleve query sorted by score.
	searchResult, hydratedDocs, err := b.searchAndHydrate(candidateQuery, b.maxSimilarityCandidates, 0, nil)
	if err != nil {
		return result.Paginated[[]Document]{}, err
	}

	if searchResult.Total == 0 || len(searchResult.Hits) == 0 {
		return result.Paginated[[]Document]{}, nil
	}

	bestScore := searchResult.Hits[0].Score
	if scoringQuery != nil {
		bestScore, err = b.bestScore(scoringQuery)
		if err != nil {
			return result.Paginated[[]Document]{}, err
		}
		if bestScore == 0 {
			return result.Paginated[[]Document]{}, nil
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

	return result.Paginate(resultsPerPage, page, len(docs), docs), nil
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

// similarityHit pairs a hydrated Document with the raw Bleve score of the match it
// came from, so sortSimilarityResults can use the score as a sort key without
// re-querying Bleve.
type similarityHit struct {
	doc   Document
	score float64
}

// sortSimilarityResults re-sorts hits (already pruned and, going in, ordered by
// -_score) according to sortBy - the same field-name syntax parseDocumentSortBy
// produces for Bleve's own SortBy, since these are the only fields it ever
// generates (see documentSortOptions). "_score"/"-_score" compares the raw Bleve
// score carried in each hit, since that isn't a Document field
// compareDocumentsBySortKey can read.
//
// If sortBy doesn't fully order two hits (e.g. it's empty, or made up of fields
// like Series/SeriesIndex that are blank for most documents), the tie is broken by
// distance between each document's publication date and referenceDate, closest
// first, so documents from around the same time as the reference document rank
// above equally-ranked ones from further away. If referenceDate is zero (the
// reference document has no publication date), that tiebreak is skipped.
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

// CountDocuments returns the total number of documents matching the given search fields, without fetching any hits.
func (b *BleveIndexer) CountDocuments(searchFields SearchFields) (int, error) {
	r, err := b.Search(searchFields, 1, 0)
	if err != nil {
		return 0, err
	}
	return r.TotalHits(), nil
}

// TotalDocs returns the number of indexed documents
func (b *BleveIndexer) TotalDocs() (uint64, error) {
	b.documentsMu.RLock()
	defer b.documentsMu.RUnlock()
	return b.documentsIdx.DocCount()
}

func (b *BleveIndexer) Document(slug string) (Document, error) {
	query := bleve.NewTermQuery(slug)
	query.SetField("Slug")

	searchOptions := bleve.NewSearchRequest(query)
	searchOptions.Fields = []string{"*"}
	b.documentsMu.RLock()
	searchResult, err := b.documentsIdx.Search(searchOptions)
	b.documentsMu.RUnlock()
	if err != nil {
		return Document{}, err
	}
	if searchResult.Total == 0 {
		return Document{}, nil
	}

	return hydrateDocument(searchResult.Hits[0]), nil
}

func (b *BleveIndexer) documentByIndexID(id string) (Document, error) {
	query := bleve.NewDocIDQuery([]string{id})
	searchOptions := bleve.NewSearchRequest(query)
	searchOptions.Fields = []string{"*"}
	b.documentsMu.RLock()
	searchResult, err := b.documentsIdx.Search(searchOptions)
	b.documentsMu.RUnlock()
	if err != nil {
		return Document{}, err
	}
	if searchResult.Total == 0 {
		return Document{}, nil
	}

	return hydrateDocument(searchResult.Hits[0]), nil
}

// IndexedFile holds the bytes and metadata for a document download.
type IndexedFile struct {
	Document    Document
	Data        []byte
	FileName    string
	ContentType string
}

// File returns the raw document payload and metadata for the given slug.
func (b *BleveIndexer) File(slug string) (*IndexedFile, error) {
	doc, err := b.Document(slug)
	if err != nil || doc.ID == "" {
		return nil, ErrDocumentNotFound
	}
	fullPath := filepath.Join(b.libraryPath, doc.ID)
	exists, err := afero.Exists(b.fs, fullPath)
	if err != nil || !exists {
		return nil, errors.New("document file not found")
	}
	data, err := afero.ReadFile(b.fs, fullPath)
	if err != nil {
		return nil, err
	}
	ext := strings.ToLower(filepath.Ext(doc.ID))
	result := &IndexedFile{
		Document:    doc,
		Data:        data,
		FileName:    filepath.Base(doc.ID),
		ContentType: "application/pdf",
	}
	if ext == ".epub" {
		result.ContentType = "application/epub+zip"
	}
	return result, nil
}

// Cover returns the cover image for the document identified by slug, resized to at most coverMaxWidth pixels wide.
func (b *BleveIndexer) Cover(slug string, coverMaxWidth int) (image.Image, error) {
	doc, err := b.Document(slug)
	if err != nil || doc.ID == "" {
		return nil, errors.New("document not found")
	}
	fullPath := filepath.Join(b.libraryPath, doc.ID)
	ext := strings.ToLower(filepath.Ext(doc.ID))
	reader, ok := b.reader[ext]
	if !ok {
		return nil, errors.New("unsupported document type for cover")
	}
	return reader.Cover(fullPath, coverMaxWidth)
}

// Documents returns documents for the given slugs in a single search. Missing or invalid slugs are omitted.
func (b *BleveIndexer) Documents(slugs []string) (map[string]Document, error) {
	if len(slugs) == 0 {
		return map[string]Document{}, nil
	}
	queries := make([]query.Query, 0, len(slugs))
	for _, slug := range slugs {
		if slug == "" {
			continue
		}
		q := bleve.NewTermQuery(slug)
		q.SetField("Slug")
		queries = append(queries, q)
	}
	if len(queries) == 0 {
		return map[string]Document{}, nil
	}
	disq := bleve.NewDisjunctionQuery(queries...)
	searchOptions := bleve.NewSearchRequest(disq)
	searchOptions.Fields = []string{"*"}
	searchOptions.Size = len(slugs)
	b.documentsMu.RLock()
	searchResult, err := b.documentsIdx.Search(searchOptions)
	b.documentsMu.RUnlock()
	if err != nil {
		return nil, err
	}
	out := make(map[string]Document, len(searchResult.Hits))
	for _, hit := range searchResult.Hits {
		if d := hydrateDocument(hit); d.Slug != "" {
			out[d.Slug] = d
		}
	}
	return out, nil
}

// TotalWordCount returns the sum of word counts for the documents matching the given slugs.
func (b *BleveIndexer) TotalWordCount(slugs []string) (float64, error) {
	docs, err := b.Documents(slugs)
	if err != nil {
		return 0, err
	}
	var totalWords float64
	for _, slug := range slugs {
		if slug == "" {
			continue
		}
		if doc, ok := docs[slug]; ok {
			totalWords += doc.Words
		}
	}
	return totalWords, nil
}

func (b *BleveIndexer) analyzers() ([]string, error) {
	// Get all languages from indexed documents (already normalized to two-letter codes)
	allLanguages, err := b.Languages()
	if err != nil {
		return []string{}, err
	}

	// Filter to only include languages that have analyzers configured
	// This is needed because composeQuery() uses these analyzers to build search queries
	analyzers := []string{}
	for _, lang := range allLanguages {
		if _, hasAnalyzer := noStopWordsFilters[lang]; hasAnalyzer {
			analyzers = append(analyzers, lang)
		}
	}

	// Always include defaultAnalyzer to ensure documents without language or with unsupported languages are searchable
	if !slices.Contains(analyzers, defaultAnalyzer) {
		analyzers = append(analyzers, defaultAnalyzer)
	}

	return analyzers, nil
}

// Languages returns a list of all unique languages in the indexed documents using faceted search.
func (b *BleveIndexer) Languages() ([]string, error) {
	if b.documentsIdx == nil {
		return []string{}, nil
	}

	// Use faceted search to get all unique languages from documents
	matchAllQuery := bleve.NewMatchAllQuery()
	searchRequest := bleve.NewSearchRequest(matchAllQuery)
	searchRequest.Size = 0 // We don't need document hits, only facets

	// Add facet request for Language field
	// Use a large size to get all unique languages
	languageFacet := bleve.NewFacetRequest("Language", 10000)
	searchRequest.AddFacet("languages", languageFacet)

	b.documentsMu.RLock()
	searchResult, err := b.documentsIdx.Search(searchRequest)
	b.documentsMu.RUnlock()
	if err != nil {
		return []string{}, err
	}

	// Use a map to track unique normalized language codes
	languageMap := make(map[string]bool)

	// Extract and normalize languages from facet results
	if languageFacetResult, ok := searchResult.Facets["languages"]; ok && languageFacetResult.Terms != nil {
		for _, term := range languageFacetResult.Terms.Terms() {
			if term.Term == "" || term.Term == "default_analyzer" {
				continue
			}
			// Normalize to two-letter base language code
			if len(term.Term) >= 2 {
				baseLang := term.Term[:2]
				languageMap[baseLang] = true
			}
		}
	}

	// Convert map to slice
	languages := make([]string, 0, len(languageMap))
	for lang := range languageMap {
		languages = append(languages, lang)
	}

	// Sort for consistent output
	slices.Sort(languages)

	return languages, nil
}

// Formats returns the distinct document formats present in the index (e.g. "epub", "pdf"), using
// faceted search. Callers can use it to decide whether to show format-specific search filters,
// such as hiding the pages filter when the library has no PDFs.
func (b *BleveIndexer) Formats() ([]string, error) {
	if b.documentsIdx == nil {
		return []string{}, nil
	}

	matchAllQuery := bleve.NewMatchAllQuery()
	searchRequest := bleve.NewSearchRequest(matchAllQuery)
	searchRequest.Size = 0 // We don't need document hits, only facets

	formatsFacet := bleve.NewFacetRequest("Format", 10)
	searchRequest.AddFacet("formats", formatsFacet)

	b.documentsMu.RLock()
	searchResult, err := b.documentsIdx.Search(searchRequest)
	b.documentsMu.RUnlock()
	if err != nil {
		return []string{}, err
	}

	formats := []string{}
	if formatsFacetResult, ok := searchResult.Facets["formats"]; ok && formatsFacetResult.Terms != nil {
		for _, term := range formatsFacetResult.Terms.Terms() {
			if term.Term != "" {
				formats = append(formats, term.Term)
			}
		}
	}

	slices.Sort(formats)
	return formats, nil
}

// normalizeSubjectName normalizes a subject name to have only the first letter capitalized.
// For example: "Science Fiction" -> "Science fiction", "SCIENCE FICTION" -> "Science fiction"
func normalizeSubjectName(subject string) string {
	if subject == "" {
		return subject
	}
	// Convert to lowercase first, then capitalize only the first letter
	subject = strings.ToLower(subject)
	if len(subject) > 0 {
		// Capitalize first letter
		first := strings.ToUpper(string(subject[0]))
		if len(subject) > 1 {
			return first + subject[1:]
		}
		return first
	}
	return subject
}

// Subjects returns subject groups: each slug with all display names that map to it.
// Uses Subjects field for faceting; names are normalized (first letter capitalized).
// Grouping uses slug.Make so variants like "cronica" and "crónica" share one slug (slug transliterates accents).
func (b *BleveIndexer) Subjects() (map[string][]string, error) {
	if b.documentsIdx == nil {
		return map[string][]string{}, nil
	}

	matchAllQuery := bleve.NewMatchAllQuery()
	searchRequest := bleve.NewSearchRequest(matchAllQuery)
	searchRequest.Size = 0
	subjectsFacet := bleve.NewFacetRequest("Subjects", 10000)
	searchRequest.AddFacet("subjects", subjectsFacet)

	b.documentsMu.RLock()
	searchResult, err := b.documentsIdx.Search(searchRequest)
	b.documentsMu.RUnlock()
	if err != nil {
		return nil, err
	}

	// slug -> unique normalized names
	bySlug := make(map[string][]string)

	if subjectsFacetResult, ok := searchResult.Facets["subjects"]; ok && subjectsFacetResult.Terms != nil {
		for _, term := range subjectsFacetResult.Terms.Terms() {
			if term.Term == "" {
				continue
			}
			normalized := normalizeSubjectName(term.Term)
			s := slug.Make(term.Term)
			if !slices.Contains(bySlug[s], normalized) {
				bySlug[s] = append(bySlug[s], normalized)
			}
		}
	}

	return bySlug, nil
}

func (b *BleveIndexer) SearchByAuthor(searchFields SearchFields, page, resultsPerPage int) (result.Paginated[[]Document], error) {
	return b.runPaginatedQuery(documentQueryByAuthorSlug(searchFields.Keywords), page, resultsPerPage, searchFields.SortBy)
}

func documentQueryByAuthorSlug(authorSlug string) query.Query {
	byAuthor := bleve.NewTermQuery(authorSlug)
	byAuthor.SetField("AuthorsSlugs")
	byIllustrator := bleve.NewTermQuery(authorSlug)
	byIllustrator.SetField("IllustratorsSlugs")
	return bleve.NewDisjunctionQuery(byAuthor, byIllustrator)
}

func (b *BleveIndexer) SearchBySeries(searchFields SearchFields, page, resultsPerPage int) (result.Paginated[[]Document], error) {
	aq := bleve.NewTermQuery(searchFields.Keywords)
	aq.SetField("SeriesSlug")

	return b.runPaginatedQuery(aq, page, resultsPerPage, searchFields.SortBy)
}

func (b *BleveIndexer) LatestDocs(limit int) ([]Document, error) {
	falseValue := false
	trueValue := true
	dateQuery := bleve.NewDateRangeInclusiveQuery(time.Time{}, time.Now().UTC(), &falseValue, &trueValue)
	dateQuery.SetField("AddedOn")

	return b.runQuery(dateQuery, limit, []string{"-AddedOn"})
}

func hydrateDocument(match *search.DocumentMatch) Document {
	var err error

	addedOn := time.Time{}
	if match.Fields["AddedOn"] != nil {
		if addedOn, err = time.Parse(time.RFC3339, match.Fields["AddedOn"].(string)); err != nil {
			return Document{}
		}
	}

	publication := precisiondate.PrecisionDate{Date: date.Zero}
	if match.Fields["Publication.Date"] != nil {
		publication.Date = date.Date(match.Fields["Publication.Date"].(float64))
		publication.Precision = match.Fields["Publication.Precision"].(float64)
	}

	language := ""
	if match.Fields["Language"] != nil {
		language = match.Fields["Language"].(string)
	}

	illustrations := 0
	if match.Fields["Illustrations"] != nil {
		illustrations = int(match.Fields["Illustrations"].(float64))
	}

	illustrators := slicer(match.Fields["Illustrators"])
	if len(illustrators) == 0 {
		illustrators = nil
	}

	illustratorsSlugs := slicer(match.Fields["IllustratorsSlugs"])
	if len(illustratorsSlugs) == 0 {
		illustratorsSlugs = nil
	}

	textRankKeywords := slicer(match.Fields["TextRankKeywords"])
	if len(textRankKeywords) == 0 {
		textRankKeywords = nil
	}

	// Absent for documents indexed before this field existed, which are
	// therefore correctly treated as not yet enriched (its zero value).
	textRankEnriched := false
	if match.Fields["TextRankEnriched"] != nil {
		textRankEnriched = match.Fields["TextRankEnriched"].(bool)
	}

	doc := Document{
		ID: match.ID,
		Metadata: metadata.Metadata{
			Title:         match.Fields["Title"].(string),
			Authors:       slicer(match.Fields["Authors"]),
			Illustrators:  illustrators,
			Description:   template.HTML(match.Fields["Description"].(string)),
			Language:      language,
			Publication:   publication,
			Words:         match.Fields["Words"].(float64),
			Series:        match.Fields["Series"].(string),
			SeriesIndex:   match.Fields["SeriesIndex"].(float64),
			Pages:         match.Fields["Pages"].(float64),
			Subjects:      slicer(match.Fields["Subjects"]),
			Illustrations: illustrations,
			Format:        match.Fields["Format"].(string),
		},
		Slug:              match.Fields["Slug"].(string),
		AuthorsSlugs:      slicer(match.Fields["AuthorsSlugs"]),
		IllustratorsSlugs: illustratorsSlugs,
		SeriesSlug:        match.Fields["SeriesSlug"].(string),
		SubjectsSlugs:     slicer(match.Fields["SubjectsSlugs"]),
		AddedOn:           addedOn,
		TextRankKeywords:  textRankKeywords,
		TextRankEnriched:  textRankEnriched,
	}

	return doc
}

func slicer(val any) []string {
	var (
		terms []any
		ok    bool
	)

	if val == nil {
		return []string{}
	}

	// Bleve indexes string slices of one element as just string
	if terms, ok = val.([]any); !ok {
		terms = append(terms, val)
	}
	termsStrings := make([]string, len(terms))
	for j, term := range terms {
		if term == nil {
			return termsStrings
		}
		termsStrings[j] = term.(string)
	}

	return termsStrings
}

// SameSubjects returns an array of metadata of documents by other authors,
// which have similar subjects as the passed one and does not belong to the same collection.
// They are sorted by subjects matching score, ties broken by closeness to the
// publication date of the reference document - the same ranking used by
// SearchFields.SimilarTo, so both surfaces show related documents in the same order.
func (b *BleveIndexer) SameSubjects(slugID string, quantity int) ([]Document, error) {
	doc, err := b.Document(slugID)
	if err != nil {
		return []Document{}, err
	}

	if len(doc.Subjects) == 0 && len(doc.TextRankKeywords) == 0 {
		return []Document{}, nil
	}

	bq := b.subjectsQuery(doc)
	paginated, err := b.runSimilarityQuery(nil, bq, 1, quantity, DefaultDocumentSortBy, float64(doc.Publication.Date))
	if err != nil {
		return []Document{}, err
	}

	return paginated.Hits(), nil
}

func (b *BleveIndexer) subjectsQuery(doc Document) *query.BooleanQuery {
	bq := bleve.NewBooleanQuery()
	subjectsCompoundQuery := bleve.NewDisjunctionQuery()

	for _, slug := range doc.SubjectsSlugs {
		qu := bleve.NewTermQuery(slug)
		qu.SetField("SubjectsSlugs")
		subjectsCompoundQuery.AddQuery(qu)
	}

	// A document can also qualify as "related" by sharing a TextRank word
	// pair or single word extracted at indexing time (EPUB only), rather
	// than an exact subject term - one MatchPhraseQuery per entry, since each
	// is stored as its own array entry (see the Document.TextRankKeywords
	// doc comment), so a pair only ever matches an actual adjacent pair in
	// the candidate document, never two words from unrelated pairs (a single
	// word entry is just a one-term phrase, so it matches normally).
	// Documents that match on both subjects and keywords, or on more shared
	// entries, score higher naturally, since DisjunctionQuery sums the
	// scores of matching clauses.
	//
	// TextRankKeywords has no upper bound, and a long or repetitive document
	// can end up with hundreds of entries (a real one observed while
	// diagnosing slow similarity queries had 782) - ORing all of them together
	// is expensive for Bleve to evaluate regardless of how common any single
	// keyword is, since it has to poll every one of those clauses for every
	// candidate document. Capped at Config.MaxSimilarityKeywords to bound that
	// cost; since TextRankKeywords is stored ordered by descending TextRank
	// weight (see textRankKeywords), taking a prefix keeps the keywords most
	// representative of the document, not an arbitrary subset. A cap of 0
	// means uncapped, matching Config.MinOccurrenceRatio's "0 disables this"
	// convention elsewhere in this same Config struct.
	keywords := doc.TextRankKeywords
	if b.maxSimilarityKeywords > 0 && len(keywords) > b.maxSimilarityKeywords {
		keywords = keywords[:b.maxSimilarityKeywords]
	}
	for _, keyword := range keywords {
		kq := bleve.NewMatchPhraseQuery(keyword)
		kq.SetField("TextRankKeywords")
		kq.Analyzer = defaultAnalyzer
		subjectsCompoundQuery.AddQuery(kq)
	}

	if doc.SeriesSlug != "" {
		sq := bleve.NewTermQuery(doc.SeriesSlug)
		sq.SetField("SeriesSlug")
		bq.AddMustNot(sq)
	}

	bq.AddMust(subjectsCompoundQuery)
	bq.AddMustNot(bleve.NewDocIDQuery([]string{doc.ID}))

	authorsCompoundQuery := bleve.NewDisjunctionQuery()
	for _, slug := range doc.AuthorsSlugs {
		qa := bleve.NewTermQuery(slug)
		qa.SetField("AuthorsSlugs")
		authorsCompoundQuery.AddQuery(qa)
	}
	bq.AddMustNot(authorsCompoundQuery)

	return bq
}

// SameAuthors returns an array of metadata of documents by the same authors which
// does not belong to the same collection
func (b *BleveIndexer) SameAuthors(slugID string, quantity int) ([]Document, error) {
	doc, err := b.Document(slugID)
	if err != nil {
		return []Document{}, err
	}

	if len(doc.Authors) == 0 {
		return []Document{}, err
	}

	authorsCompoundQuery := bleve.NewDisjunctionQuery()
	for _, slug := range doc.AuthorsSlugs {
		qu := bleve.NewTermQuery(slug)
		qu.SetField("AuthorsSlugs")
		authorsCompoundQuery.AddQuery(qu)
	}
	bq := bleve.NewBooleanQuery()
	bq.AddMust(authorsCompoundQuery)
	bq.AddMustNot(bleve.NewDocIDQuery([]string{doc.ID}))

	if doc.Series != "" {
		sq := bleve.NewTermQuery(doc.SeriesSlug)
		sq.SetField("SeriesSlug")
		bq.AddMustNot(sq)
	}

	return b.runQuery(bq, quantity, []string{"-_score", "Series", "SeriesIndex"})
}

// SameSeries returns an array of metadata of documents in the same series
func (b *BleveIndexer) SameSeries(slugID string, quantity int) ([]Document, error) {
	doc, err := b.Document(slugID)
	if err != nil {
		return []Document{}, err
	}

	if doc.Series == "" {
		return []Document{}, err
	}

	bq := bleve.NewBooleanQuery()
	bq.AddMustNot(bleve.NewDocIDQuery([]string{doc.ID}))

	sq := bleve.NewTermQuery(doc.SeriesSlug)
	sq.SetField("SeriesSlug")
	bq.AddMust(sq)

	return b.runQuery(bq, quantity, []string{"-_score", "Series", "SeriesIndex"})
}
