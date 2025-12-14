package index

import (
	"html/template"
	"io/fs"
	"math"
	"net/url"
	"slices"
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
	searchResult, err := b.documentsIdx.Search(searchOptions)
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
func (b *BleveIndexer) Count() (uint64, error) {
	matchAllQuery := bleve.NewMatchAllQuery()

	searchRequest := bleve.NewSearchRequest(matchAllQuery)
	searchResult, err := b.documentsIdx.Search(searchRequest)
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
	searchResult, err := b.documentsIdx.Search(searchOptions)
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
	searchResult, err := b.documentsIdx.Search(searchOptions)
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
	searchResult, err := b.documentsIdx.Search(searchOptions)
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
	searchResult, err := b.documentsIdx.Search(searchOptions)
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
	// Get all languages from indexed documents
	allLanguages, err := b.Languages()
	if err != nil {
		return []string{}, err
	}

	// Filter to only include languages that have analyzers configured
	// This is needed because composeQuery() uses these analyzers to build search queries
	// Normalize language codes to two letters for analyzer lookup
	analyzers := []string{}
	seenAnalyzers := make(map[string]bool)
	for _, lang := range allLanguages {
		// Normalize to two-letter code for analyzer lookup
		normalizedLang := lang
		if len(lang) >= 2 {
			normalizedLang = lang[:2]
		}
		if _, hasAnalyzer := noStopWordsFilters[normalizedLang]; hasAnalyzer {
			// Deduplicate normalized analyzers
			if !seenAnalyzers[normalizedLang] {
				analyzers = append(analyzers, normalizedLang)
				seenAnalyzers[normalizedLang] = true
			}
		}
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

	searchResult, err := b.documentsIdx.Search(searchRequest)
	if err != nil {
		return []string{}, err
	}

	languages := []string{}

	// Extract languages from facet results
	if languageFacetResult, ok := searchResult.Facets["languages"]; ok && languageFacetResult.Terms != nil {
		for _, term := range languageFacetResult.Terms.Terms() {
			if term.Term == "" || term.Term == "default_analyzer" {
				continue
			}
			languages = append(languages, term.Term)
		}
	}

	// Sort for consistent output
	slices.Sort(languages)

	return languages, nil
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

// hydrateAuthorFromFields converts a fields map to an Author struct
// This is the shared implementation used by hydrateAuthor
func hydrateAuthorFromFields(fields map[string]interface{}, docID string) Author {
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
