package index

import (
	"fmt"
	"html/template"
	"math"
	"strings"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/svera/coreander/v3/internal/controller"
	"github.com/svera/coreander/v3/internal/metadata"
)

// Search look for documents which match with the passed keywords. Returns a maximum <resultsPerPage> books, offset by <page>
func (b *BleveIndexer) Search(keywords string, page, resultsPerPage int, wordsPerMinute float64) (*controller.Result, error) {
	for _, prefix := range []string{"Authors:", "Series:", "Title:", "Subjects:", "\""} {
		if strings.HasPrefix(strings.Trim(keywords, " "), prefix) {
			query := bleve.NewQueryStringQuery(keywords)

			return b.runQuery(query, page, resultsPerPage, wordsPerMinute)
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
	return b.runQuery(compound, page, resultsPerPage, wordsPerMinute)
}

func (b *BleveIndexer) runQuery(query query.Query, page, resultsPerPage int, wordsPerMinute float64) (*controller.Result, error) {
	var result controller.Result
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
	result = controller.Result{
		Page:       page,
		TotalPages: totalPages,
		TotalHits:  int(searchResult.Total),
		Hits:       make([]metadata.Metadata, len(searchResult.Hits)),
	}

	for i, val := range searchResult.Hits {
		doc := metadata.Metadata{
			ID:          val.ID,
			Slug:        val.Fields["Slug"].(string),
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
			ReadingTime: calculateReadingTime(val.Fields["Words"].(float64), wordsPerMinute),
		}
		result.Hits[i] = doc
	}
	return &result, nil
}

func calculateReadingTime(words, wordsPerMinute float64) string {
	if words == 0.0 {
		return ""
	}
	readingTime, err := time.ParseDuration(fmt.Sprintf("%fm", words/wordsPerMinute))
	if err != nil {
		return ""
	}
	return fmtDuration(readingTime)
}

func fmtDuration(d time.Duration) string {
	d = d.Round(time.Minute)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	return fmt.Sprintf("%dh %dm", h, m)
}

// Count returns the number of indexed books
func (b *BleveIndexer) Count() (uint64, error) {
	return b.idx.DocCount()
}

func calculateTotalPages(total, resultsPerPage uint64) int {
	return int(math.Ceil(float64(total) / float64(resultsPerPage)))
}

func (b *BleveIndexer) Document(slug string) (metadata.Metadata, error) {
	doc := metadata.Metadata{}
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

	doc = metadata.Metadata{
		ID:          searchResult.Hits[0].ID,
		Slug:        searchResult.Hits[0].Fields["Slug"].(string),
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
	}

	return doc, nil
}

func slicer(val interface{}) []string {
	var (
		terms []interface{}
		ok    bool
	)

	// Bleve indexes string slices of one element as just string
	if terms, ok = val.([]interface{}); !ok {
		terms = append(terms, val)
	}
	termsStrings := make([]string, len(terms))
	for j, term := range terms {
		if term == nil {
			return []string{""}
		}
		termsStrings[j] = term.(string)
	}

	return termsStrings
}
