package index

import (
	"cmp"
	"slices"
	"strings"
	"unicode"

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

func (b *BleveIndexer) runAuthorsPaginatedQuery(q query.Query, page, resultsPerPage int, sortBy []string) (result.Paginated[[]Author], error) {
	if page < 1 {
		page = 1
	}

	if desc, ok := authorDocumentCountSortDesc(sortBy); ok {
		countResult, err := b.authorsIdx.Search(bleve.NewSearchRequestOptions(q, 0, 0, false))
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

		authors := hydrateAuthors(searchResult.Hits)
		counts, err := b.DocumentCountsByAuthorSlugs(authorSlugsFromAuthors(authors))
		if err != nil {
			return result.Paginated[[]Author]{}, err
		}
		sortAuthorsByDocumentCount(authors, counts, desc)
		return result.Paginate(resultsPerPage, page, total, authors), nil
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

	return result.Paginate(
		resultsPerPage,
		page,
		int(searchResult.Total),
		hydrateAuthors(searchResult.Hits),
	), nil
}

func authorDocumentCountSortDesc(sortBy []string) (desc bool, ok bool) {
	if len(sortBy) != 1 {
		return false, false
	}
	switch sortBy[0] {
	case "DocumentCount":
		return false, true
	case "-DocumentCount":
		return true, true
	default:
		return false, false
	}
}

func hydrateAuthors(hits search.DocumentMatchCollection) []Author {
	authors := make([]Author, len(hits))
	for i, hit := range hits {
		authors[i] = hydrateAuthor(hit)
	}
	return authors
}

func authorSlugsFromAuthors(authors []Author) []string {
	slugs := make([]string, len(authors))
	for i, author := range authors {
		slugs[i] = author.Slug
	}
	return slugs
}

func sortAuthorsByDocumentCount(authors []Author, counts map[string]uint64, desc bool) {
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
}
