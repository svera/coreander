package index

import (
	"cmp"
	"slices"
	"strings"
	"unicode"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/rickb777/date/v2"
	"github.com/svera/coreander/v4/internal/result"
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

	if q := authorNameQuery(searchFields.Name); q != nil {
		filtersQuery.AddQuery(q)
	} else {
		filtersQuery.AddQuery(bleve.NewMatchAllQuery())
	}

	addAuthorFilters(searchFields, filtersQuery)

	return b.runAuthorsPaginatedQuery(filtersQuery, page, resultsPerPage, searchFields.SortBy)
}

func authorNameQuery(name string) query.Query {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	name = foldAuthorName(name)

	disj := bleve.NewDisjunctionQuery()
	for _, field := range []string{"Name", "BirthName"} {
		prefix := bleve.NewPrefixQuery(name)
		prefix.SetField(field)
		disj.AddQuery(prefix)

		wildcard := bleve.NewWildcardQuery("*" + escapeWildcard(name) + "*")
		wildcard.SetField(field)
		disj.AddQuery(wildcard)
	}
	return disj
}

func foldAuthorName(name string) string {
	return strings.Map(func(r rune) rune {
		return unicode.ToLower(r)
	}, name)
}

func escapeWildcard(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `*`, `\*`, `?`, `\?`)
	return replacer.Replace(value)
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

func addDateRangeFilter(filtersQuery *query.ConjunctionQuery, field string, from, to date.Date) {
	if from == 0 && to == 0 {
		return
	}
	minDate := float64(from)
	q := bleve.NewNumericRangeQuery(nil, nil)
	if from != 0 {
		q.Min = &minDate
	}
	if to != 0 {
		maxDate := float64(to) + 1
		q.Max = &maxDate
	}
	q.SetField(field)
	filtersQuery.AddQuery(q)
}

func (b *BleveIndexer) runAuthorsPaginatedQuery(q query.Query, page, resultsPerPage int, sortBy []string) (result.Paginated[[]Author], error) {
	if page < 1 {
		page = 1
	}

	if documentCountSort, desc := authorDocumentCountSort(sortBy); documentCountSort {
		return b.runAuthorsPaginatedQueryByDocumentCount(q, page, resultsPerPage, desc)
	}

	searchOptions := bleve.NewSearchRequestOptions(q, resultsPerPage, (page-1)*resultsPerPage, false)
	searchOptions.SortBy(sortBy)
	searchOptions.Fields = []string{"*"}
	searchResult, err := b.authorsIdx.Search(searchOptions)
	if err != nil {
		return result.Paginated[[]Author]{}, err
	}
	if searchResult.Total == 0 {
		return result.Paginated[[]Author]{}, nil
	}

	authors := make([]Author, len(searchResult.Hits))
	for i, hit := range searchResult.Hits {
		authors[i] = hydrateAuthor(hit)
	}

	return result.NewPaginated(
		resultsPerPage,
		page,
		int(searchResult.Total),
		authors,
	), nil
}

func authorDocumentCountSort(sortBy []string) (bool, bool) {
	if len(sortBy) != 1 {
		return false, false
	}
	switch sortBy[0] {
	case "DocumentCount":
		return true, false
	case "-DocumentCount":
		return true, true
	default:
		return false, false
	}
}

func (b *BleveIndexer) runAuthorsPaginatedQueryByDocumentCount(q query.Query, page, resultsPerPage int, desc bool) (result.Paginated[[]Author], error) {
	countRequest := bleve.NewSearchRequestOptions(q, 0, 0, false)
	countResult, err := b.authorsIdx.Search(countRequest)
	if err != nil {
		return result.Paginated[[]Author]{}, err
	}
	total := int(countResult.Total)
	if total == 0 {
		return result.Paginated[[]Author]{}, nil
	}

	searchOptions := bleve.NewSearchRequestOptions(q, total, 0, false)
	searchOptions.Fields = []string{"*"}
	searchResult, err := b.authorsIdx.Search(searchOptions)
	if err != nil {
		return result.Paginated[[]Author]{}, err
	}

	authors := make([]Author, len(searchResult.Hits))
	slugs := make([]string, len(searchResult.Hits))
	for i, hit := range searchResult.Hits {
		authors[i] = hydrateAuthor(hit)
		slugs[i] = authors[i].Slug
	}

	counts, err := b.DocumentCountsByAuthorSlugs(slugs)
	if err != nil {
		return result.Paginated[[]Author]{}, err
	}

	slices.SortFunc(authors, func(a, b Author) int {
		countA := counts[a.Slug]
		countB := counts[b.Slug]
		if countA != countB {
			if desc {
				return cmp.Compare(countB, countA)
			}
			return cmp.Compare(countA, countB)
		}
		return strings.Compare(a.Name, b.Name)
	})

	start := (page - 1) * resultsPerPage
	if start >= total {
		return result.NewPaginated(resultsPerPage, page, total, []Author{}), nil
	}
	end := start + resultsPerPage
	if end > total {
		end = total
	}

	return result.NewPaginated(
		resultsPerPage,
		page,
		total,
		authors[start:end],
	), nil
}
