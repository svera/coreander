package index

import (
	"fmt"
	"html/template"
	"math"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/gosimple/slug"
	"github.com/svera/coreander/v3/internal/metadata"
)

// PaginatedResult holds the result of a search request, as well as some related metadata
type PaginatedResult struct {
	Page       int
	TotalPages int
	Hits       []Document
	TotalHits  int
}

// Search look for documents which match with the passed keywords. Returns a maximum <resultsPerPage> books, offset by <page>
func (b *BleveIndexer) Search(keywords string, page, resultsPerPage int) (*PaginatedResult, error) {
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
		if strings.HasPrefix(unescaped, prefix) {
			unescaped = strings.Replace(unescaped, prefix, "", 1)
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
	}

	splitted := strings.Split(strings.TrimSpace(keywords), " ")

	var (
		authorQueries      []query.Query
		titleQueries       []query.Query
		descriptionQueries []query.Query
		seriesQueries      []query.Query
		subjectQueries     []query.Query
	)

	for _, keyword := range splitted {
		if keyword == "" {
			continue
		}
		qa := bleve.NewMatchQuery(keyword)
		qa.SetField("Authors")
		authorQueries = append(authorQueries, qa)

		qt := bleve.NewMatchQuery(keyword)
		qt.SetField("Title")
		titleQueries = append(titleQueries, qt)

		qs := bleve.NewMatchQuery(keyword)
		qs.SetField("Series")
		seriesQueries = append(seriesQueries, qs)

		qu := bleve.NewMatchQuery(keyword)
		qu.SetField("Subjects")
		subjectQueries = append(subjectQueries, qt)

		qd := bleve.NewMatchQuery(keyword)
		qd.SetField("Description")
		descriptionQueries = append(descriptionQueries, qd)
	}

	authorCompoundQuery := bleve.NewConjunctionQuery(authorQueries...)
	titleCompoundQuery := bleve.NewConjunctionQuery(titleQueries...)
	seriesCompoundQuery := bleve.NewConjunctionQuery(seriesQueries...)
	descriptionCompoundQuery := bleve.NewConjunctionQuery(descriptionQueries...)
	subjectCompoundQuery := bleve.NewConjunctionQuery(subjectQueries...)

	compound := bleve.NewDisjunctionQuery(authorCompoundQuery, titleCompoundQuery, seriesCompoundQuery, descriptionCompoundQuery, subjectCompoundQuery)
	return b.runPaginatedQuery(compound, page, resultsPerPage)
}

func (b *BleveIndexer) runQuery(query query.Query, results int) ([]Document, error) {
	res, err := b.runPaginatedQuery(query, 0, results)
	if err != nil {
		return nil, err
	}
	return res.Hits, nil
}

func (b *BleveIndexer) runPaginatedQuery(query query.Query, page, resultsPerPage int) (*PaginatedResult, error) {
	var result PaginatedResult
	if page < 1 {
		page = 1
	}

	searchOptions := bleve.NewSearchRequestOptions(query, resultsPerPage, (page-1)*resultsPerPage, false)
	searchOptions.SortBy([]string{"-_score", "Series", "SeriesIndex"})
	searchOptions.Fields = []string{"ID", "Slug", "Title", "Authors", "Description", "Year", "Words", "Series", "SeriesIndex", "Pages", "Type", "Subjects"}
	searchResult, err := b.idx.Search(searchOptions)
	if err != nil {
		return nil, err
	}
	if searchResult.Total == 0 {
		return &result, nil
	}
	totalPages := calculateTotalPages(searchResult.Total, uint64(resultsPerPage))
	if totalPages < page {
		page = totalPages
		if page == 0 {
			page = 1
		}
		searchResult, err = b.idx.Search(searchOptions)
		if err != nil {
			return nil, err
		}
	}
	result = PaginatedResult{
		Page:       page,
		TotalPages: totalPages,
		TotalHits:  int(searchResult.Total),
		Hits:       make([]Document, 0, len(searchResult.Hits)),
	}

	for _, val := range searchResult.Hits {
		doc := Document{
			ID:       val.ID,
			BaseName: filepath.Base(val.ID),
			Slug:     val.Fields["Slug"].(string),
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
		result.Hits = append(result.Hits, doc)
	}
	return &result, nil
}

// Count returns the number of indexed books
func (b *BleveIndexer) Count() (uint64, error) {
	return b.idx.DocCount()
}

func calculateTotalPages(total, resultsPerPage uint64) int {
	return int(math.Ceil(float64(total) / float64(resultsPerPage)))
}

func (b *BleveIndexer) Document(slug string) (Document, error) {
	doc := Document{}
	query := bleve.NewTermQuery(slug)
	query.SetField("Slug")
	searchOptions := bleve.NewSearchRequest(query)
	searchOptions.Fields = []string{"ID", "Slug", "Title", "Authors", "Description", "Year", "Words", "Series", "SeriesIndex", "Pages", "Type", "Subjects"}
	searchResult, err := b.idx.Search(searchOptions)
	if err != nil {
		return doc, err
	}
	if searchResult.Total == 0 {
		return doc, fmt.Errorf("Document with slug %s not found", slug)
	}

	doc = Document{
		ID:       searchResult.Hits[0].ID,
		BaseName: filepath.Base(searchResult.Hits[0].ID),
		Slug:     searchResult.Hits[0].Fields["Slug"].(string),
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
	}

	return doc, nil
}

// SameSubjects returns an array of metadata of documents by other authors, different between each other,
// which have similar subjects as the passed one and does not belong to the same collection
func (b *BleveIndexer) SameSubjects(slug string, quantity int) ([]Document, error) {
	doc, err := b.Document(slug)
	if err != nil {
		return []Document{}, err
	}

	subjectsCompoundQuery := bleve.NewDisjunctionQuery()
	for _, subject := range doc.Subjects {
		qu := bleve.NewMatchPhraseQuery(subject)
		qu.SetField("Subjects")
		subjectsCompoundQuery.AddQuery(qu)
	}
	bq := bleve.NewBooleanQuery()
	bq.AddMust(subjectsCompoundQuery)
	bq.AddMustNot(bleve.NewDocIDQuery([]string{doc.ID}))
	sq := bleve.NewMatchPhraseQuery(doc.Series)
	sq.SetField("Series")
	bq.AddMustNot(sq)
	authorsCompoundQuery := bleve.NewDisjunctionQuery()
	for _, author := range doc.Authors {
		qa := bleve.NewMatchPhraseQuery(author)
		qa.SetField("Authors")
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
			qa := bleve.NewMatchPhraseQuery(author)
			qa.SetField("Authors")
			authorsCompoundQuery.AddQuery(qa)
		}
		bq.AddMustNot(authorsCompoundQuery)
	}

	return res, err
}

// SameAuthors returns an array of metadata of documents by the same authors which
// does not belong to the same collection
func (b *BleveIndexer) SameAuthors(slug string, quantity int) ([]Document, error) {
	doc, err := b.Document(slug)
	if err != nil {
		return []Document{}, err
	}

	authorsCompoundQuery := bleve.NewDisjunctionQuery()
	for _, author := range doc.Authors {
		qu := bleve.NewMatchPhraseQuery(author)
		qu.SetField("Authors")
		authorsCompoundQuery.AddQuery(qu)
	}
	bq := bleve.NewBooleanQuery()
	bq.AddMust(authorsCompoundQuery)
	bq.AddMustNot(bleve.NewDocIDQuery([]string{doc.ID}))
	sq := bleve.NewMatchPhraseQuery(doc.Series)
	sq.SetField("Series")
	bq.AddMustNot(sq)

	return b.runQuery(bq, quantity)
}

// SameSeries returns an array of metadata of documents in the same series
func (b *BleveIndexer) SameSeries(slug string, quantity int) ([]Document, error) {
	doc, err := b.Document(slug)
	if err != nil {
		return []Document{}, err
	}

	bq := bleve.NewBooleanQuery()
	bq.AddMustNot(bleve.NewDocIDQuery([]string{doc.ID}))
	sq := bleve.NewMatchPhraseQuery(doc.Series)
	sq.SetField("Series")
	bq.AddMust(sq)

	return b.runQuery(bq, quantity)
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
