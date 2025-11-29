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
				filtersQuery.AddQuery(query)
				addFilters(searchFields, filtersQuery)

				return b.runPaginatedQuery(filtersQuery, page, resultsPerPage, searchFields.SortBy)
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
			filtersQuery.AddQuery(qb)
			addFilters(searchFields, filtersQuery)
			return b.runPaginatedQuery(filtersQuery, page, resultsPerPage, searchFields.SortBy)
		}

		analyzers, err := b.analyzers()
		if err != nil {
			return result.Paginated[[]Document]{}, err
		}

		query := composeQuery(searchFields.Keywords, analyzers)
		filtersQuery.AddQuery(query)
	}

	addFilters(searchFields, filtersQuery)

	return b.runPaginatedQuery(filtersQuery, page, resultsPerPage, searchFields.SortBy)
}

func addFilters(searchFields SearchFields, filtersQuery *query.ConjunctionQuery) {
	if searchFields.Language != "" {
		q := bleve.NewTermQuery(searchFields.Language)
		q.SetField("Language")
		filtersQuery.AddQuery(q)
	}
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
	if searchFields.EstReadTimeFrom > 0 || searchFields.EstReadTimeTo > 0 {
		q := bleve.NewNumericRangeQuery(nil, nil)
		if searchFields.EstReadTimeFrom > 0 {
			min := searchFields.EstReadTimeFrom * 60 * searchFields.WordsPerMinute
			q.Min = &min
		}
		if searchFields.EstReadTimeTo > 0 {
			max := searchFields.EstReadTimeTo * 60 * searchFields.WordsPerMinute
			q.Max = &max
		}
		q.SetField("Words")
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
// The t parameter is deprecated (documents and authors are now in separate indexes)
func (b *BleveIndexer) Count(t string) (uint64, error) {
	matchAllQuery := bleve.NewMatchAllQuery()

	searchRequest := bleve.NewSearchRequest(matchAllQuery)
	searchResult, err := b.idx.Search(searchRequest)
	if err != nil {
		return 0, err
	}
	return searchResult.Total, nil
}

func (b *BleveIndexer) Document(slug string) (Document, error) {
	query := bleve.NewTermQuery(slug)
	query.SetField("Slug")

	searchOptions := bleve.NewSearchRequest(query)
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

func (b *BleveIndexer) DocumentByID(ID string) (Document, error) {
	query := bleve.NewDocIDQuery([]string{ID})

	searchOptions := bleve.NewSearchRequest(query)
	searchOptions.Fields = []string{"*"}
	searchOptions.Size = 1
	searchResult, err := b.idx.Search(searchOptions)
	if err != nil {
		return Document{}, err
	}

	if searchResult.Total == 0 {
		return Document{}, nil
	}

	return hydrateDocument(searchResult.Hits[0]), nil
}

func (b *BleveIndexer) Documents(IDs []string, sortBy []string) ([]Document, error) {
	var docs []Document
	query := bleve.NewDocIDQuery(IDs)

	searchOptions := bleve.NewSearchRequest(query)
	searchOptions.Fields = []string{"*"}
	searchOptions.SortBy(sortBy)
	searchResult, err := b.idx.Search(searchOptions)
	if err != nil {
		return docs, err
	}

	for _, hit := range searchResult.Hits {
		docs = append(docs, hydrateDocument(hit))
	}

	return docs, nil
}

// TotalWordCount returns the sum of word counts for the given document IDs
func (b *BleveIndexer) TotalWordCount(IDs []string) (float64, error) {
	if len(IDs) == 0 {
		return 0, nil
	}

	query := bleve.NewDocIDQuery(IDs)

	searchOptions := bleve.NewSearchRequest(query)
	searchOptions.Fields = []string{"Words"}
	searchOptions.Size = len(IDs)
	searchResult, err := b.idx.Search(searchOptions)
	if err != nil {
		return 0, err
	}

	var totalWords float64
	for _, hit := range searchResult.Hits {
		if hit.Fields["Words"] != nil {
			totalWords += hit.Fields["Words"].(float64)
		}
	}

	return totalWords, nil
}

func (b *BleveIndexer) analyzers() ([]string, error) {
	languages, err := b.idx.GetInternal([]byte("languages"))
	if err != nil {
		return []string{}, err
	}
	return strings.Split(string(languages), ","), nil
}

// Languages returns a list of all unique languages in the indexed documents
func (b *BleveIndexer) Languages() ([]string, error) {
	languages, err := b.idx.GetInternal([]byte("languages"))
	if err != nil {
		return []string{}, err
	}
	if len(languages) == 0 {
		return []string{}, nil
	}

	allLanguages := strings.Split(string(languages), ",")
	var filteredLanguages []string
	for _, lang := range allLanguages {
		// Filter out empty strings and default_analyzer
		if lang != "" && lang != "default_analyzer" {
			filteredLanguages = append(filteredLanguages, lang)
		}
	}

	return filteredLanguages, nil
}

func (b *BleveIndexer) SearchByAuthor(searchFields SearchFields, page, resultsPerPage int) (result.Paginated[[]Document], error) {
	aq := bleve.NewTermQuery(searchFields.Keywords)
	aq.SetField("AuthorsSlugs")

	return b.runPaginatedQuery(aq, page, resultsPerPage, searchFields.SortBy)
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

	doc := Document{
		ID: match.ID,
		Metadata: metadata.Metadata{
			Title:       match.Fields["Title"].(string),
			Authors:     slicer(match.Fields["Authors"]),
			Description: template.HTML(match.Fields["Description"].(string)),
			Language:    language,
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

// MigrateAuthors migrates all authors from an old authors index to a new one
func MigrateAuthors(oldIndex, newIndex bleve.Index) error {
	matchAllQuery := bleve.NewMatchAllQuery()
	searchRequest := bleve.NewSearchRequest(matchAllQuery)
	searchRequest.Size = 10000 // Process in batches
	searchRequest.Fields = []string{"*"}

	batch := newIndex.NewBatch()
	batchCount := 0

	for {
		searchResult, err := oldIndex.Search(searchRequest)
		if err != nil {
			return err
		}

		if searchResult.Total == 0 {
			break
		}

		// Migrate each author
		for _, hit := range searchResult.Hits {
			author := hydrateAuthor(hit)
			if author.Slug == "" {
				continue
			}

			if err := batch.Index(author.Slug, author); err != nil {
				return err
			}
			batchCount++

			// Execute batch every 1000 items
			if batchCount >= 1000 {
				if err := newIndex.Batch(batch); err != nil {
					return err
				}
				batch = newIndex.NewBatch()
				batchCount = 0
			}
		}

		// If we got less than requested size, we're done
		if len(searchResult.Hits) < searchRequest.Size {
			break
		}

		// Move to next batch
		searchRequest.From += searchRequest.Size
	}

	// Execute any remaining authors
	if batchCount > 0 {
		if err := newIndex.Batch(batch); err != nil {
			return err
		}
	}

	return nil
}

func hydrateAuthor(hit *search.DocumentMatch) Author {
	retrievedOn := time.Time{}
	if hit.Fields["RetrievedOn"] != nil {
		if t, err := time.Parse("2006-01-02T15:04:05Z", hit.Fields["RetrievedOn"].(string)); err == nil {
			retrievedOn = t
		}
	}

	dateOfBirth := precisiondate.PrecisionDate{Date: date.Zero}
	if hit.Fields["DateOfBirth.Date"] != nil {
		dateOfBirth.Date = date.Date(hit.Fields["DateOfBirth.Date"].(float64))
		if hit.Fields["DateOfBirth.Precision"] != nil {
			dateOfBirth.Precision = hit.Fields["DateOfBirth.Precision"].(float64)
		}
	}

	dateOfDeath := precisiondate.PrecisionDate{Date: date.Zero}
	if hit.Fields["DateOfDeath.Date"] != nil {
		dateOfDeath.Date = date.Date(hit.Fields["DateOfDeath.Date"].(float64))
		if hit.Fields["DateOfDeath.Precision"] != nil {
			dateOfDeath.Precision = hit.Fields["DateOfDeath.Precision"].(float64)
		}
	}

	name := ""
	if hit.Fields["Name"] != nil {
		name = hit.Fields["Name"].(string)
	}

	birthName := ""
	if hit.Fields["BirthName"] != nil {
		birthName = hit.Fields["BirthName"].(string)
	}

	slug := hit.ID
	if hit.Fields["Slug"] != nil {
		slug = hit.Fields["Slug"].(string)
	}

	dataSourceID := ""
	if hit.Fields["DataSourceID"] != nil {
		dataSourceID = hit.Fields["DataSourceID"].(string)
	}

	website := ""
	if hit.Fields["Website"] != nil {
		website = hit.Fields["Website"].(string)
	}

	dataSourceImage := ""
	if hit.Fields["DataSourceImage"] != nil {
		dataSourceImage = hit.Fields["DataSourceImage"].(string)
	}

	instanceOf := float64(0)
	if hit.Fields["InstanceOf"] != nil {
		instanceOf = hit.Fields["InstanceOf"].(float64)
	}

	gender := float64(0)
	if hit.Fields["Gender"] != nil {
		gender = hit.Fields["Gender"].(float64)
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
		Pseudonyms:      slicer(hit.Fields["Pseudonyms"]),
	}

	// Extract Wikipedia links and descriptions for all languages
	for key, value := range hit.Fields {
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
