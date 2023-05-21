package index

import (
	"fmt"
	"html/template"
	"math"
	"strings"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/svera/coreander/v2/internal/controller"
	"github.com/svera/coreander/v2/internal/metadata"
)

// Search look for documents which match with the passed keywords. Returns a maximum <resultsPerPage> books, offset by <page>
func (b *BleveIndexer) Search(keywords string, page, resultsPerPage int, wordsPerMinute float64) (*controller.Result, error) {
	for _, prefix := range []string{"Authors:", "Series:", "Title:", "\""} {
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

		qd := bleve.NewMatchQuery(keyword)
		qd.SetField("Description")
		descriptionQueries = append(descriptionQueries, qd)
	}

	authorCompoundQuery := bleve.NewConjunctionQuery(authorQueries...)
	titleCompoundQuery := bleve.NewConjunctionQuery(titleQueries...)
	seriesCompoundQuery := bleve.NewConjunctionQuery(seriesQueries...)
	descriptionCompoundQuery := bleve.NewConjunctionQuery(descriptionQueries...)

	compound := bleve.NewDisjunctionQuery(authorCompoundQuery, titleCompoundQuery, seriesCompoundQuery, descriptionCompoundQuery)
	return b.runQuery(compound, page, resultsPerPage, wordsPerMinute)
}

func (b *BleveIndexer) runQuery(query query.Query, page, resultsPerPage int, wordsPerMinute float64) (*controller.Result, error) {
	var result controller.Result
	if page < 1 {
		page = 1
	}

	searchOptions := bleve.NewSearchRequestOptions(query, resultsPerPage, (page-1)*resultsPerPage, false)
	searchOptions.SortBy([]string{"-_score", "Series", "SeriesIndex"})
	searchOptions.Fields = []string{"Filename", "Title", "Authors", "Description", "Year", "Words", "Series", "SeriesIndex", "Pages", "Type"}
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
			Filename:    val.Fields["Filename"].(string),
			Title:       val.Fields["Title"].(string),
			Authors:     authors(val),
			Description: template.HTML(val.Fields["Description"].(string)),
			Year:        val.Fields["Year"].(string),
			Words:       val.Fields["Words"].(float64),
			Series:      val.Fields["Series"].(string),
			SeriesIndex: val.Fields["SeriesIndex"].(float64),
			Pages:       int(val.Fields["Pages"].(float64)),
			Type:        val.Fields["Type"].(string),
		}
		if doc.Words != 0.0 {
			readingTime, err := time.ParseDuration(fmt.Sprintf("%fm", doc.Words/wordsPerMinute))
			if err == nil {
				doc.ReadingTime = fmtDuration(readingTime)
			}
		}
		result.Hits[i] = doc
	}
	return &result, nil
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

func (b *BleveIndexer) Document(ID string) (metadata.Metadata, error) {
	doc := metadata.Metadata{}
	query := bleve.NewDocIDQuery([]string{ID})
	searchOptions := bleve.NewSearchRequestOptions(query, 1, 0, false)
	searchOptions.Fields = []string{"Filename", "Title", "Authors", "Description", "Year", "Words", "Series", "SeriesIndex", "Pages", "Type"}
	searchResult, err := b.idx.Search(searchOptions)
	if err != nil {
		return doc, err
	}
	if searchResult.Total == 0 {
		return doc, fmt.Errorf("Document not found")
	}

	doc = metadata.Metadata{
		ID:          searchResult.Hits[0].ID,
		Filename:    searchResult.Hits[0].Fields["Filename"].(string),
		Title:       searchResult.Hits[0].Fields["Title"].(string),
		Authors:     authors(searchResult.Hits[0]),
		Description: template.HTML(searchResult.Hits[0].Fields["Description"].(string)),
		Year:        searchResult.Hits[0].Fields["Year"].(string),
		Words:       searchResult.Hits[0].Fields["Words"].(float64),
		Series:      searchResult.Hits[0].Fields["Series"].(string),
		SeriesIndex: searchResult.Hits[0].Fields["SeriesIndex"].(float64),
		Pages:       int(searchResult.Hits[0].Fields["Pages"].(float64)),
		Type:        searchResult.Hits[0].Fields["Type"].(string),
	}

	return doc, nil
}

func authors(val *search.DocumentMatch) []string {
	var (
		authors []interface{}
		ok      bool
	)

	// Bleve indexes string slices of one element as just string
	if authors, ok = val.Fields["Authors"].([]interface{}); !ok {
		authors = append(authors, val.Fields["Authors"])
	}
	authorsStrings := make([]string, len(authors))
	for j, author := range authors {
		authorsStrings[j] = author.(string)
	}

	return authorsStrings
}
