package index

import (
	"html/template"
	"io/fs"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/rickb777/date/v2"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v4/internal/metadata"
	"github.com/svera/coreander/v4/internal/precisiondate"
	"github.com/svera/coreander/v4/internal/result"
)

func (b *BleveIndexer) IndexingProgress() (Progress, error) {
	var progress Progress

	if b.indexStartTime == 0 {
		return progress, nil
	}
	elapsedTime := float64(time.Now().UnixNano()) - b.indexStartTime
	libraryFiles, err := countFiles(b.libraryPath, b.fs)
	if err != nil {
		return progress, err
	}
	progress.RemainingTime = time.Duration((elapsedTime * (libraryFiles - b.indexedEntries)) / b.indexedEntries)
	progress.Percentage = math.Round((100 / libraryFiles) * b.indexedEntries)
	return progress, nil
}

func countFiles(dir string, fileSystem afero.Fs) (float64, error) {
	var total float64

	afero.Walk(fileSystem, dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		total++
		return nil
	})
	return total, nil
}

// Search look for documents which match the passed keywords and filters.
// Returns a maximum <resultsPerPage> documents, offset by <page>
func (b *BleveIndexer) Search(searchFields SearchFields, page, resultsPerPage int) (result.Paginated[[]Document], error) {
	filtersQuery := bleve.NewConjunctionQuery()

	if searchFields.Keywords != "" {
		for _, prefix := range []string{"Authors:", "Series:", "Title:", "Subjects:", "\""} {
			if strings.HasPrefix(strings.Trim(searchFields.Keywords, " "), prefix) {
				query := bleve.NewQueryStringQuery(searchFields.Keywords)

				return b.runPaginatedQuery(query, page, resultsPerPage, []string{"-_score", "Series", "SeriesIndex"})
			}
		}

		for _, prefix := range []string{"AuthorsSlugs:", "SeriesSlug:", "SubjectsSlugs:"} {
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
			return b.runPaginatedQuery(qb, page, resultsPerPage, []string{"-_score", "Series", "SeriesIndex"})
		}

		analyzers, err := b.analyzers()
		if err != nil {
			return result.Paginated[[]Document]{}, err
		}

		query := composeQuery(searchFields.Keywords, analyzers)
		filtersQuery.AddQuery(query)
	}

	addFilters(searchFields, filtersQuery)

	return b.runPaginatedQuery(filtersQuery, page, resultsPerPage, []string{"-_score", "Series", "SeriesIndex"})
}

func addFilters(searchFields SearchFields, filtersQuery *query.ConjunctionQuery) {
	if searchFields.PubDateFrom != 0 || searchFields.PubDateTo != 0 {
		minDate := float64(searchFields.PubDateFrom)
		maxDate := float64(searchFields.PubDateTo)

		q := bleve.NewNumericRangeQuery(nil, nil)
		if minDate != 0 {
			q.Min = &minDate
		}
		if maxDate != 0 {
			q.Max = &maxDate
		}
		q.SetField("Publication.Date")
		filtersQuery.AddQuery(q)
	}
}

func composeQuery(keywords string, analyzers []string) *query.DisjunctionQuery {
	langCompoundQuery := bleve.NewDisjunctionQuery()
	// Special query for searches using partial title names and author names
	authorTitleQuery := bleve.NewConjunctionQuery()
	allLangsOrTitleQuery := bleve.NewDisjunctionQuery()

	for _, analyzer := range analyzers {
		noStopWordsAnalyzer := analyzer
		if analyzer != defaultAnalyzer && analyzer != "" {
			noStopWordsAnalyzer = analyzer + "_no_stop_words"
		}

		qt := bleve.NewMatchQuery(keywords)
		qt.Analyzer = noStopWordsAnalyzer
		qt.SetField("Title")
		qt.Operator = query.MatchQueryOperatorAnd

		qs := bleve.NewMatchQuery(keywords)
		qs.Analyzer = noStopWordsAnalyzer
		qs.SetField("Series")
		qs.Operator = query.MatchQueryOperatorAnd

		qu := bleve.NewMatchQuery(keywords)
		qu.Analyzer = analyzer
		qu.SetField("Subjects")
		qu.Operator = query.MatchQueryOperatorAnd

		qd := bleve.NewMatchQuery(keywords)
		qd.Analyzer = analyzer
		qd.SetField("Description")
		qd.Operator = query.MatchQueryOperatorAnd

		langCompoundQuery.AddQuery(qt, qs, qu, qd)

		orTitleQuery := bleve.NewMatchQuery(keywords)
		orTitleQuery.SetField("Title")
		orTitleQuery.Operator = query.MatchQueryOperatorOr
		orTitleQuery.Analyzer = analyzer

		allLangsOrTitleQuery.AddQuery(orTitleQuery)
	}

	qa := bleve.NewMatchQuery(keywords)
	qa.SetField("Authors")
	qa.Operator = query.MatchQueryOperatorAnd
	qa.Analyzer = defaultAnalyzer

	orAuthorQuery := bleve.NewMatchQuery(keywords)
	orAuthorQuery.SetField("Authors")
	orAuthorQuery.Operator = query.MatchQueryOperatorOr
	orAuthorQuery.Analyzer = defaultAnalyzer

	authorTitleQuery.AddQuery(orAuthorQuery, allLangsOrTitleQuery)

	return bleve.NewDisjunctionQuery(qa, langCompoundQuery, authorTitleQuery)
}

func (b *BleveIndexer) runQuery(query query.Query, results int, sortBy []string) ([]Document, error) {
	res, err := b.runPaginatedQuery(query, 0, results, sortBy)
	if err != nil {
		return nil, err
	}
	return res.Hits(), nil
}

func (b *BleveIndexer) runPaginatedQuery(query query.Query, page, resultsPerPage int, sortBy []string) (result.Paginated[[]Document], error) {
	var res result.Paginated[[]Document]

	if page < 1 {
		page = 1
	}

	searchOptions := bleve.NewSearchRequestOptions(query, resultsPerPage, (page-1)*resultsPerPage, false)
	searchOptions.SortBy(sortBy)
	searchOptions.Fields = []string{"*"}
	searchResult, err := b.idx.Search(searchOptions)
	if err != nil {
		return result.Paginated[[]Document]{}, err
	}

	if searchResult.Total == 0 {
		return res, nil
	}

	docs := make([]Document, len(searchResult.Hits))

	for i, val := range searchResult.Hits {
		docs[i] = hydrateDocument(val)
	}

	return result.NewPaginated(
		resultsPerPage,
		page,
		int(searchResult.Total),
		docs,
	), nil
}

// Count returns the number of indexed documents
func (b *BleveIndexer) Count(t string) (uint64, error) {
	tq := bleve.NewTermQuery(t)
	tq.SetField("Type")

	searchRequest := bleve.NewSearchRequest(tq)
	searchResult, err := b.idx.Search(searchRequest)
	if err != nil {
		return 0, err
	}
	return searchResult.Total, nil
}

func (b *BleveIndexer) Document(slug string) (Document, error) {
	compoundQuery := bleve.NewConjunctionQuery()
	query := bleve.NewTermQuery(slug)
	query.SetField("Slug")
	typeQuery := bleve.NewTermQuery(TypeDocument)
	typeQuery.SetField("Type")
	compoundQuery.AddQuery(query, typeQuery)

	searchOptions := bleve.NewSearchRequest(compoundQuery)
	searchOptions.Fields = []string{"*"}
	searchResult, err := b.idx.Search(searchOptions)
	if err != nil {
		return Document{}, err
	}
	if searchResult.Total == 0 {
		return Document{}, nil
	}

	return hydrateDocument(searchResult.Hits[0]), nil
}

func (b *BleveIndexer) Documents(IDs []string) (map[string]Document, error) {
	compoundQuery := bleve.NewConjunctionQuery()
	docs := make(map[string]Document, len(IDs))
	query := bleve.NewDocIDQuery(IDs)
	typeQuery := bleve.NewTermQuery(TypeDocument)
	typeQuery.SetField("Type")
	compoundQuery.AddQuery(query, typeQuery)

	searchOptions := bleve.NewSearchRequest(compoundQuery)
	searchOptions.Fields = []string{"*"}
	searchResult, err := b.idx.Search(searchOptions)
	if err != nil {
		return docs, err
	}

	for _, hit := range searchResult.Hits {
		docs[hit.ID] = hydrateDocument(hit)
	}

	return docs, nil
}

func (b *BleveIndexer) analyzers() ([]string, error) {
	languages, err := b.idx.GetInternal([]byte("languages"))
	if err != nil {
		return []string{}, err
	}
	return strings.Split(string(languages), ","), nil
}

func (b *BleveIndexer) SearchByAuthor(authorSlug string, page, resultsPerPage int) (result.Paginated[[]Document], error) {
	aq := bleve.NewTermQuery(authorSlug)
	aq.SetField("AuthorsSlugs")

	return b.runPaginatedQuery(aq, page, resultsPerPage, []string{"Series", "SeriesIndex"})
}

func (b *BleveIndexer) Author(slug, lang string) (Author, error) {
	authorsCompoundQuery := bleve.NewConjunctionQuery()

	aq := bleve.NewTermQuery(slug)
	aq.SetField("Slug")
	authorsCompoundQuery.AddQuery(aq)

	tq := bleve.NewTermQuery(TypeAuthor)
	tq.SetField("Type")
	authorsCompoundQuery.AddQuery(tq)

	searchOptions := bleve.NewSearchRequest(authorsCompoundQuery)
	searchOptions.Fields = []string{"*"}
	searchResult, err := b.idx.Search(searchOptions)
	if err != nil {
		return Author{}, err
	}
	if searchResult.Total == 0 {
		return Author{}, nil
	}

	retrievedOn := time.Time{}
	if searchResult.Hits[0].Fields["RetrievedOn"] != nil {
		retrievedOn, err = time.Parse("2006-01-02T15:04:05Z", searchResult.Hits[0].Fields["RetrievedOn"].(string))
		if err != nil {
			return Author{}, err
		}
	}
	dateOfBirth := precisiondate.PrecisionDate{Date: date.Zero}
	if searchResult.Hits[0].Fields["DateOfBirth.Date"] != nil {
		dateOfBirth.Date = date.Date(searchResult.Hits[0].Fields["DateOfBirth.Date"].(float64))
		dateOfBirth.Precision = searchResult.Hits[0].Fields["DateOfBirth.Precision"].(float64)
	}
	dateOfDeath := precisiondate.PrecisionDate{Date: date.Zero}
	if searchResult.Hits[0].Fields["DateOfDeath.Date"] != nil {
		dateOfDeath.Date = date.Date(searchResult.Hits[0].Fields["DateOfDeath.Date"].(float64))
		dateOfDeath.Precision = searchResult.Hits[0].Fields["DateOfDeath.Precision"].(float64)
	}

	author := Author{
		Name:            searchResult.Hits[0].Fields["Name"].(string),
		BirthName:       searchResult.Hits[0].Fields["BirthName"].(string),
		Slug:            slug,
		DataSourceID:    searchResult.Hits[0].Fields["DataSourceID"].(string),
		RetrievedOn:     retrievedOn,
		WikipediaLink:   make(map[string]string),
		InstanceOf:      searchResult.Hits[0].Fields["InstanceOf"].(float64),
		Description:     make(map[string]string),
		DateOfBirth:     dateOfBirth,
		DateOfDeath:     dateOfDeath,
		Website:         searchResult.Hits[0].Fields["Website"].(string),
		DataSourceImage: searchResult.Hits[0].Fields["DataSourceImage"].(string),
		Gender:          searchResult.Hits[0].Fields["Gender"].(float64),
		Pseudonyms:      slicer(searchResult.Hits[0].Fields["Pseudonyms"]),
	}

	if value, ok := searchResult.Hits[0].Fields["WikipediaLink."+lang].(string); ok {
		author.WikipediaLink[lang] = value
	}
	if value, ok := searchResult.Hits[0].Fields["Description."+lang].(string); ok {
		author.Description[lang] = value
	}

	return author, nil
}

func (b *BleveIndexer) SearchBySeries(seriesSlug string, page, resultsPerPage int) (result.Paginated[[]Document], error) {
	aq := bleve.NewTermQuery(seriesSlug)
	aq.SetField("SeriesSlug")

	return b.runPaginatedQuery(aq, page, resultsPerPage, []string{"SeriesIndex"})
}

func (b *BleveIndexer) LatestDocs(limit int) ([]Document, error) {
	bq := bleve.NewBooleanQuery()

	typeQuery := bleve.NewTermQuery(TypeDocument)
	typeQuery.SetField("Type")

	falseValue := false
	trueValue := true
	dateQuery := bleve.NewDateRangeInclusiveQuery(time.Time{}, time.Now().UTC(), &falseValue, &trueValue)
	dateQuery.SetField("AddedOn")

	bq.AddMust(typeQuery)
	bq.AddMust(dateQuery)

	return b.runQuery(bq, limit, []string{"-AddedOn"})
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

	doc := Document{
		ID: match.ID,
		Metadata: metadata.Metadata{
			Title:       match.Fields["Title"].(string),
			Authors:     slicer(match.Fields["Authors"]),
			Description: template.HTML(match.Fields["Description"].(string)),
			Publication: publication,
			Words:       match.Fields["Words"].(float64),
			Series:      match.Fields["Series"].(string),
			SeriesIndex: match.Fields["SeriesIndex"].(float64),
			Pages:       match.Fields["Pages"].(float64),
			Subjects:    slicer(match.Fields["Subjects"]),
			Format:      match.Fields["Format"].(string),
		},
		Slug:          match.Fields["Slug"].(string),
		AuthorsSlugs:  slicer(match.Fields["AuthorsSlugs"]),
		SeriesSlug:    match.Fields["SeriesSlug"].(string),
		SubjectsSlugs: slicer(match.Fields["SubjectsSlugs"]),
		AddedOn:       addedOn,
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
