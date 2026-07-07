package index

import (
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/rickb777/date/v2"
	"github.com/svera/coreander/v5/internal/result"
)

type AuthorSearchFields struct {
	Name          string
	Gender        *float64
	BirthDateFrom date.Date
	BirthDateTo   date.Date
	DeathDateFrom date.Date
	DeathDateTo   date.Date
	SortBy        []string
}

func (b *BleveIndexer) SearchAuthors(searchFields AuthorSearchFields, page, resultsPerPage int) (result.Paginated[[]Author], error) {
	filtersQuery := bleve.NewConjunctionQuery()

	if q := b.authorNameQuery(searchFields.Name); q != nil {
		filtersQuery.AddQuery(q)
	} else {
		filtersQuery.AddQuery(bleve.NewMatchAllQuery())
	}

	addAuthorFilters(searchFields, filtersQuery)

	return b.runAuthorsPaginatedQuery(filtersQuery, page, resultsPerPage, searchFields.SortBy)
}

func (b *BleveIndexer) authorNameQuery(name string) query.Query {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}

	disj := bleve.NewDisjunctionQuery()

	for _, field := range []string{"Name", "BirthName"} {
		match := bleve.NewMatchQuery(name)
		match.SetField(field)
		match.Analyzer = defaultAnalyzer
		match.Operator = query.MatchQueryOperatorAnd
		disj.AddQuery(match)
	}
	return disj
}

func addAuthorFilters(searchFields AuthorSearchFields, filtersQuery *query.ConjunctionQuery) {
	if searchFields.Gender != nil {
		min := *searchFields.Gender
		max := min + 1
		q := bleve.NewNumericRangeQuery(&min, &max)
		q.SetField("Gender")
		filtersQuery.AddQuery(q)
	}
	addDateRangeFilter(filtersQuery, "DateOfBirth.Date", searchFields.BirthDateFrom, searchFields.BirthDateTo)
	addDeathDateRangeFilter(filtersQuery, searchFields.DeathDateFrom, searchFields.DeathDateTo)
}

func addDeathDateRangeFilter(filtersQuery *query.ConjunctionQuery, from, to date.Date) {
	if from == 0 && to == 0 {
		return
	}
	addDateRangeFilter(filtersQuery, "DateOfDeath.Date", from, to)

	// Living authors are indexed with DateOfDeath.Date == 0, which otherwise matches
	// an open-ended "died before" filter because 0 <= max.
	zero := float64(0)
	one := float64(1)
	maxExclusive := false
	zeroDeathDate := bleve.NewNumericRangeInclusiveQuery(&zero, &one, nil, &maxExclusive)
	zeroDeathDate.SetField("DateOfDeath.Date")
	excludeLiving := bleve.NewBooleanQuery()
	excludeLiving.AddMustNot(zeroDeathDate)
	filtersQuery.AddQuery(excludeLiving)
}

// CountAuthors returns the total number of authors matching the given search fields, without fetching any hits.
func (b *BleveIndexer) CountAuthors(searchFields AuthorSearchFields) (int, error) {
	r, err := b.SearchAuthors(searchFields, 1, 0)
	if err != nil {
		return 0, err
	}
	return r.TotalHits(), nil
}

func (b *BleveIndexer) runAuthorsPaginatedQuery(q query.Query, page, resultsPerPage int, sortBy []string) (result.Paginated[[]Author], error) {
	if page < 1 {
		page = 1
	}

	searchOptions := bleve.NewSearchRequestOptions(q, resultsPerPage, (page-1)*resultsPerPage, false)
	// See the equivalent comment in runPaginatedQuery: only override the default relevance sort
	// when the caller actually asked for a specific order.
	if len(sortBy) > 0 {
		searchOptions.SortBy(sortBy)
	}
	searchOptions.Fields = []string{"*"}
	searchResult, err := b.authorsIdx.Search(searchOptions)
	if err != nil {
		return result.Paginated[[]Author]{}, err
	}
	if searchResult.Total == 0 {
		return result.Paginated[[]Author]{}, nil
	}

	return result.Paginate(
		resultsPerPage,
		page,
		int(searchResult.Total),
		hydrateAuthors(searchResult.Hits),
	), nil
}

func hydrateAuthors(hits search.DocumentMatchCollection) []Author {
	authors := make([]Author, len(hits))
	for i, hit := range hits {
		authors[i] = hydrateAuthor(hit)
	}
	return authors
}
