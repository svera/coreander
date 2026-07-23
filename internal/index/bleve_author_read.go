package index

import (
	"strings"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/rickb777/date/v2"
	"github.com/svera/coreander/v5/internal/precisiondate"
	"github.com/svera/coreander/v5/internal/result"
)

// TotalAuthors returns the number of indexed authors.
func (b *BleveIndexer) TotalAuthors() (uint64, error) {
	return b.authorsIdx.DocCount()
}

func (b *BleveIndexer) Author(slug, lang string) (Author, error) {
	aq := bleve.NewTermQuery(slug)
	aq.SetField("Slug")

	searchOptions := bleve.NewSearchRequest(aq)
	searchOptions.Fields = []string{"*"}
	searchResult, err := b.authorsIdx.Search(searchOptions)
	if err != nil {
		return Author{}, err
	}
	if searchResult.Total == 0 {
		return Author{}, nil
	}

	// Use the shared hydrateAuthor function
	author := hydrateAuthor(searchResult.Hits[0])

	// Override language-specific fields if requested language is available
	if value, ok := searchResult.Hits[0].Fields["WikipediaLink."+lang].(string); ok {
		author.WikipediaLink[lang] = value
	}
	if value, ok := searchResult.Hits[0].Fields["Description."+lang].(string); ok {
		author.Description[lang] = value
	}

	return author, nil
}

// hydrateAuthorFromFields converts a fields map to an Author struct
// This is the shared implementation used by hydrateAuthor
func hydrateAuthorFromFields(fields map[string]any, docID string) Author {
	retrievedOn := time.Time{}
	if val, ok := fields["RetrievedOn"]; ok && val != nil {
		if str, ok := val.(string); ok && str != "" {
			// Try RFC3339 format first (standard)
			if t, err := time.Parse(time.RFC3339, str); err == nil {
				retrievedOn = t
			} else if t, err := time.Parse("2006-01-02T15:04:05Z", str); err == nil {
				retrievedOn = t
			}
		}
	}

	dateOfBirth := precisiondate.PrecisionDate{Date: date.Zero}
	if val, ok := fields["DateOfBirth.Date"]; ok && val != nil {
		if dateVal, ok := val.(float64); ok {
			dateOfBirth.Date = date.Date(dateVal)
			if precVal, ok := fields["DateOfBirth.Precision"]; ok && precVal != nil {
				if prec, ok := precVal.(float64); ok {
					dateOfBirth.Precision = prec
				}
			}
		}
	}

	dateOfDeath := precisiondate.PrecisionDate{Date: date.Zero}
	if val, ok := fields["DateOfDeath.Date"]; ok && val != nil {
		if dateVal, ok := val.(float64); ok {
			dateOfDeath.Date = date.Date(dateVal)
			if precVal, ok := fields["DateOfDeath.Precision"]; ok && precVal != nil {
				if prec, ok := precVal.(float64); ok {
					dateOfDeath.Precision = prec
				}
			}
		}
	}

	name := ""
	if val, ok := fields["Name"]; ok && val != nil {
		if str, ok := val.(string); ok {
			name = str
		}
	}

	birthName := ""
	if val, ok := fields["BirthName"]; ok && val != nil {
		if str, ok := val.(string); ok {
			birthName = str
		}
	}

	slug := docID
	if val, ok := fields["Slug"]; ok && val != nil {
		if str, ok := val.(string); ok && str != "" {
			slug = str
		}
	}

	dataSourceID := ""
	if val, ok := fields["DataSourceID"]; ok && val != nil {
		if str, ok := val.(string); ok {
			dataSourceID = str
		}
	}

	website := ""
	if val, ok := fields["Website"]; ok && val != nil {
		if str, ok := val.(string); ok {
			website = str
		}
	}

	dataSourceImage := ""
	if val, ok := fields["DataSourceImage"]; ok && val != nil {
		if str, ok := val.(string); ok {
			dataSourceImage = str
		}
	}

	instanceOf := float64(0)
	if val, ok := fields["InstanceOf"]; ok && val != nil {
		if num, ok := val.(float64); ok {
			instanceOf = num
		}
	}

	gender := float64(0)
	if val, ok := fields["Gender"]; ok && val != nil {
		if num, ok := val.(float64); ok {
			gender = num
		}
	}

	documentCount := uint64(0)
	if val, ok := fields["DocumentCount"]; ok && val != nil {
		if num, ok := val.(float64); ok {
			documentCount = uint64(num)
		}
	}

	author := Author{
		Name:            name,
		BirthName:       birthName,
		Slug:            slug,
		DataSourceID:    dataSourceID,
		RetrievedOn:     retrievedOn,
		WikipediaLink:   make(map[string]string),
		InstanceOf:      instanceOf,
		Description:     make(map[string]string),
		DateOfBirth:     dateOfBirth,
		DateOfDeath:     dateOfDeath,
		Website:         website,
		DataSourceImage: dataSourceImage,
		Gender:          gender,
		Pseudonyms:      slicer(fields["Pseudonyms"]),
		DocumentCount:   documentCount,
	}

	// Extract Wikipedia links and descriptions for all languages
	for key, value := range fields {
		if strings.HasPrefix(key, "WikipediaLink.") {
			lang := strings.TrimPrefix(key, "WikipediaLink.")
			if str, ok := value.(string); ok {
				author.WikipediaLink[lang] = str
			}
		}
		if strings.HasPrefix(key, "Description.") {
			lang := strings.TrimPrefix(key, "Description.")
			if str, ok := value.(string); ok {
				author.Description[lang] = str
			}
		}
	}

	return author
}

func hydrateAuthor(hit *search.DocumentMatch) Author {
	// Convert search.DocumentMatch.Fields (map[string]interface{}) to the format expected by hydrateAuthorFromFields
	fields := make(map[string]interface{})
	for k, v := range hit.Fields {
		fields[k] = v
	}
	return hydrateAuthorFromFields(fields, hit.ID)
}

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

// AuthorsWithoutInfo returns indexed authors that have not been enriched from an external source yet.
func (b *BleveIndexer) AuthorsWithoutInfo() ([]Author, error) {
	count, err := b.authorsIdx.DocCount()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, nil
	}

	searchReq := bleve.NewSearchRequest(bleve.NewMatchAllQuery())
	searchReq.Fields = []string{"*"}
	searchReq.Size = int(count)

	searchResult, err := b.authorsIdx.Search(searchReq)
	if err != nil {
		return nil, err
	}

	authors := make([]Author, 0, len(searchResult.Hits))
	for _, hit := range searchResult.Hits {
		author := hydrateAuthor(hit)
		if author.RetrievedOn.IsZero() {
			authors = append(authors, author)
		}
	}
	return authors, nil
}
