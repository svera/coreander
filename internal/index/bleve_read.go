package index

import (
	"fmt"
	"html/template"
	"io/fs"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/gosimple/slug"
	"github.com/spf13/afero"
	"github.com/svera/coreander/v3/internal/metadata"
	"github.com/svera/coreander/v3/internal/result"
)

func (b *BleveIndexer) IndexingProgress() (Progress, error) {
	var progress Progress

	if b.indexStartTime == 0 {
		return progress, nil
	}
	ellapsedTime := float64(time.Now().UnixNano()) - b.indexStartTime
	libraryFiles, err := countFiles(b.libraryPath, b.fs)
	if err != nil {
		return progress, err
	}
	progress.RemainingTime = time.Duration((ellapsedTime * (libraryFiles - b.indexedDocuments)) / b.indexedDocuments)
	progress.Percentage = math.Round((100 / libraryFiles) * b.indexedDocuments)
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

// Search look for documents which match with the passed keywords. Returns a maximum <resultsPerPage> documents, offset by <page>
func (b *BleveIndexer) Search(keywords string, page, resultsPerPage int) (result.Paginated[[]Document], error) {
	for _, prefix := range []string{"Authors:", "Series:", "Title:", "Subjects:", "\""} {
		if strings.HasPrefix(strings.Trim(keywords, " "), prefix) {
			query := bleve.NewQueryStringQuery(keywords)

			return b.runPaginatedQuery(query, page, resultsPerPage)
		}
	}

	for _, prefix := range []string{"AuthorsEq:", "SeriesEq:", "SubjectsEq:"} {
		unescaped, err := url.QueryUnescape(strings.TrimSpace(keywords))
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
			term = strings.ReplaceAll(slug.Make(term), "-", "")
			qs := bleve.NewTermQuery(term)
			qs.SetField(strings.TrimSuffix(prefix, ":"))
			qb.AddQuery(qs)
		}
		return b.runPaginatedQuery(qb, page, resultsPerPage)
	}

	analyzers, err := b.analyzers()
	if err != nil {
		return result.Paginated[[]Document]{}, err
	}
	compound := composeQuery(keywords, analyzers)
	return b.runPaginatedQuery(compound, page, resultsPerPage)
}

func composeQuery(keywords string, analyzers []string) *query.DisjunctionQuery {
	langCompoundQuery := bleve.NewDisjunctionQuery()

	for _, analyzer := range analyzers {
		noStopWordsAnalyzer := analyzer
		if analyzer != defaultAnalyzer {
			noStopWordsAnalyzer = analyzer + "_no_stop_words"
		}

		qt := bleve.NewMatchPhraseQuery(keywords)
		qt.Analyzer = noStopWordsAnalyzer
		qt.SetField("Title")
		langCompoundQuery.AddQuery(qt)

		qs := bleve.NewMatchQuery(keywords)
		qs.Analyzer = noStopWordsAnalyzer
		qs.SetField("Series")
		qs.Operator = query.MatchQueryOperatorAnd
		langCompoundQuery.AddQuery(qs)

		qu := bleve.NewMatchQuery(keywords)
		qu.Analyzer = analyzer
		qu.SetField("Subjects")
		qu.Operator = query.MatchQueryOperatorAnd
		langCompoundQuery.AddQuery(qu)

		qd := bleve.NewMatchQuery(keywords)
		qd.Analyzer = analyzer
		qd.SetField("Description")
		qd.Operator = query.MatchQueryOperatorAnd
		langCompoundQuery.AddQuery(qd)
	}

	qa := bleve.NewMatchQuery(keywords)
	qa.SetField("Authors")
	qa.Operator = query.MatchQueryOperatorAnd
	qa.Analyzer = defaultAnalyzer

	return bleve.NewDisjunctionQuery(qa, langCompoundQuery)
}

func (b *BleveIndexer) runQuery(query query.Query, results int) ([]Document, error) {
	res, err := b.runPaginatedQuery(query, 0, results)
	if err != nil {
		return nil, err
	}
	return res.Hits(), nil
}

func (b *BleveIndexer) runPaginatedQuery(query query.Query, page, resultsPerPage int) (result.Paginated[[]Document], error) {
	var res result.Paginated[[]Document]

	if page < 1 {
		page = 1
	}

	searchOptions := bleve.NewSearchRequestOptions(query, resultsPerPage, (page-1)*resultsPerPage, false)
	searchOptions.SortBy([]string{"-_score", "Series", "SeriesIndex"})
	searchOptions.Fields = []string{"ID", "Slug", "Title", "Authors", "Description", "Year", "Words", "Series", "SeriesIndex", "Pages", "Type", "Subjects"}
	searchResult, err := b.idx.Search(searchOptions)
	if err != nil {
		return result.Paginated[[]Document]{}, err
	}

	if searchResult.Total == 0 {
		return res, nil
	}

	docs := make([]Document, 0, len(searchResult.Hits))

	for _, val := range searchResult.Hits {
		doc := Document{
			ID:   val.ID,
			Slug: val.Fields["Slug"].(string),
			Metadata: metadata.Metadata{
				Title:       val.Fields["Title"].(string),
				Authors:     slicer(val.Fields["Authors"]),
				Description: template.HTML(val.Fields["Description"].(string)),
				Year:        val.Fields["Year"].(string),
				Words:       val.Fields["Words"].(float64),
				Series:      val.Fields["Series"].(string),
				SeriesIndex: val.Fields["SeriesIndex"].(float64),
				Pages:       int(val.Fields["Pages"].(float64)),
				Type:        val.Fields["Type"].(string),
				Subjects:    slicer(val.Fields["Subjects"]),
			},
		}
		docs = append(docs, doc)
	}

	return result.NewPaginated[[]Document](
		resultsPerPage,
		page,
		int(searchResult.Total),
		docs,
	), nil
}

// Count returns the number of indexed documents
func (b *BleveIndexer) Count() (uint64, error) {
	return b.idx.DocCount()
}

func (b *BleveIndexer) Document(slug string) (Document, error) {
	query := bleve.NewTermQuery(slug)
	query.SetField("Slug")
	searchOptions := bleve.NewSearchRequest(query)
	searchOptions.Fields = []string{"ID", "Slug", "Title", "Authors", "Description", "Year", "Words", "Series", "SeriesIndex", "Pages", "Type", "Subjects"}
	searchResult, err := b.idx.Search(searchOptions)
	if err != nil {
		return Document{}, err
	}
	if searchResult.Total == 0 {
		return Document{}, fmt.Errorf("Document with slug %s not found", slug)
	}

	return Document{
		ID:   searchResult.Hits[0].ID,
		Slug: searchResult.Hits[0].Fields["Slug"].(string),
		Metadata: metadata.Metadata{
			Title:       searchResult.Hits[0].Fields["Title"].(string),
			Authors:     slicer(searchResult.Hits[0].Fields["Authors"]),
			Description: template.HTML(searchResult.Hits[0].Fields["Description"].(string)),
			Year:        searchResult.Hits[0].Fields["Year"].(string),
			Words:       searchResult.Hits[0].Fields["Words"].(float64),
			Series:      searchResult.Hits[0].Fields["Series"].(string),
			SeriesIndex: searchResult.Hits[0].Fields["SeriesIndex"].(float64),
			Pages:       int(searchResult.Hits[0].Fields["Pages"].(float64)),
			Type:        searchResult.Hits[0].Fields["Type"].(string),
			Subjects:    slicer(searchResult.Hits[0].Fields["Subjects"]),
		},
	}, nil
}

func (b *BleveIndexer) Documents(IDs []string) (map[string]Document, error) {
	docs := make(map[string]Document, len(IDs))
	query := bleve.NewDocIDQuery(IDs)
	searchOptions := bleve.NewSearchRequest(query)
	searchOptions.Fields = []string{"ID", "Slug", "Title", "Authors", "Description", "Year", "Words", "Series", "SeriesIndex", "Pages", "Type", "Subjects"}
	searchResult, err := b.idx.Search(searchOptions)
	if err != nil {
		return docs, err
	}

	for _, hit := range searchResult.Hits {
		docs[hit.ID] =
			Document{
				ID:   hit.ID,
				Slug: hit.Fields["Slug"].(string),
				Metadata: metadata.Metadata{
					Title:       hit.Fields["Title"].(string),
					Authors:     slicer(hit.Fields["Authors"]),
					Description: template.HTML(hit.Fields["Description"].(string)),
					Year:        hit.Fields["Year"].(string),
					Words:       hit.Fields["Words"].(float64),
					Series:      hit.Fields["Series"].(string),
					SeriesIndex: hit.Fields["SeriesIndex"].(float64),
					Pages:       int(hit.Fields["Pages"].(float64)),
					Type:        hit.Fields["Type"].(string),
					Subjects:    slicer(hit.Fields["Subjects"]),
				},
			}
	}

	return docs, nil
}

// SameSubjects returns an array of metadata of documents by other authors, different between each other,
// which have similar subjects as the passed one and does not belong to the same collection
func (b *BleveIndexer) SameSubjects(slugID string, quantity int) ([]Document, error) {
	doc, err := b.Document(slugID)
	if err != nil {
		return []Document{}, err
	}

	bq := bleve.NewBooleanQuery()
	subjectsCompoundQuery := bleve.NewDisjunctionQuery()

	for _, subject := range doc.Subjects {
		subject = strings.ReplaceAll(slug.Make(subject), "-", "")
		qu := bleve.NewTermQuery(subject)
		qu.SetField("SubjectsEq")
		subjectsCompoundQuery.AddQuery(qu)
	}

	if doc.Series != "" {
		series := strings.ReplaceAll(slug.Make(doc.Series), "-", "")
		sq := bleve.NewTermQuery(series)
		sq.SetField("SeriesEq")
		bq.AddMustNot(sq)
	}

	bq.AddMust(subjectsCompoundQuery)
	bq.AddMustNot(bleve.NewDocIDQuery([]string{doc.ID}))

	authorsCompoundQuery := bleve.NewDisjunctionQuery()
	for _, author := range doc.Authors {
		author = strings.ReplaceAll(slug.Make(author), "-", "")
		qa := bleve.NewTermQuery(author)
		qa.SetField("AuthorsEq")
		authorsCompoundQuery.AddQuery(qa)
	}
	bq.AddMustNot(authorsCompoundQuery)

	res := make([]Document, 0, quantity)
	for i := 0; i < quantity; i++ {
		doc, err := b.runQuery(bq, 1)
		if err != nil {
			return res, err
		}
		if len(doc) == 0 {
			return res, nil
		}
		res = append(res, doc[0])
		for _, author := range doc[0].Authors {
			author = strings.ReplaceAll(slug.Make(author), "-", "")
			qa := bleve.NewTermQuery(author)
			qa.SetField("AuthorsEq")
			authorsCompoundQuery.AddQuery(qa)
		}
		bq.AddMustNot(authorsCompoundQuery)
	}

	return res, err
}

// SameAuthors returns an array of metadata of documents by the same authors which
// does not belong to the same collection
func (b *BleveIndexer) SameAuthors(slugID string, quantity int) ([]Document, error) {
	doc, err := b.Document(slugID)
	if err != nil {
		return []Document{}, err
	}

	authorsCompoundQuery := bleve.NewDisjunctionQuery()
	for _, author := range doc.Authors {
		author = strings.ReplaceAll(slug.Make(author), "-", "")
		qu := bleve.NewTermQuery(author)
		qu.SetField("AuthorsEq")
		authorsCompoundQuery.AddQuery(qu)
	}
	bq := bleve.NewBooleanQuery()
	bq.AddMust(authorsCompoundQuery)
	bq.AddMustNot(bleve.NewDocIDQuery([]string{doc.ID}))

	if doc.Series != "" {
		series := strings.ReplaceAll(slug.Make(doc.Series), "-", "")
		sq := bleve.NewTermQuery(series)
		sq.SetField("SeriesEq")
		bq.AddMustNot(sq)
	}

	return b.runQuery(bq, quantity)
}

// SameSeries returns an array of metadata of documents in the same series
func (b *BleveIndexer) SameSeries(slugID string, quantity int) ([]Document, error) {
	doc, err := b.Document(slugID)
	if err != nil {
		return []Document{}, err
	}

	bq := bleve.NewBooleanQuery()
	bq.AddMustNot(bleve.NewDocIDQuery([]string{doc.ID}))
	series := strings.ReplaceAll(slug.Make(doc.Series), "-", "")

	sq := bleve.NewMatchPhraseQuery(series)
	sq.SetField("SeriesEq")
	bq.AddMust(sq)

	return b.runQuery(bq, quantity)
}

func (b *BleveIndexer) analyzers() ([]string, error) {
	languages, err := b.idx.GetInternal([]byte("languages"))
	if err != nil {
		return []string{}, err
	}
	return strings.Split(string(languages), ","), nil
}

func slicer(val interface{}) []string {
	var (
		terms []interface{}
		ok    bool
	)

	if val == nil {
		return []string{}
	}

	// Bleve indexes string slices of one element as just string
	if terms, ok = val.([]interface{}); !ok {
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
