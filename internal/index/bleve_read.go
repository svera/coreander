package index

import (
	"fmt"
	"html/template"
	"math"
	"strings"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/svera/coreander/internal/controller"
	"github.com/svera/coreander/internal/metadata"
)

// Search look for documents which match with the passed keywords. Returns a maximum <resultsPerPage> books, offset by <page>
func (b *BleveIndexer) Search(keywords string, page, resultsPerPage int, wordsPerMinute float64) (*controller.Result, error) {
	prefixes := []string{"Author:", "Series:", "Title:"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(strings.Trim(keywords, " "), prefix) {
			query := bleve.NewQueryStringQuery(keywords)

			return b.runQuery(query, page, resultsPerPage, wordsPerMinute)
		}
	}

	splitted := strings.Split(keywords, " ")

	var authorQueries []query.Query
	for _, keyword := range splitted {
		q := bleve.NewMatchQuery(keyword)
		q.SetField("Author")
		authorQueries = append(authorQueries, q)
	}
	authorCompoundQuery := bleve.NewConjunctionQuery(authorQueries...)
	authorCompoundQuery.SetBoost(10)

	var titleQueries []query.Query
	for _, keyword := range splitted {
		q := bleve.NewMatchQuery(keyword)
		q.SetField("Title")
		titleQueries = append(titleQueries, q)
	}
	titleCompoundQuery := bleve.NewConjunctionQuery(titleQueries...)
	titleCompoundQuery.SetBoost(10)

	var descriptionQueries []query.Query
	for _, keyword := range splitted {
		q := bleve.NewMatchQuery(keyword)
		q.SetField("Description")
		descriptionQueries = append(descriptionQueries, q)
	}
	descriptionCompoundQuery := bleve.NewConjunctionQuery(descriptionQueries...)

	compound := bleve.NewDisjunctionQuery(authorCompoundQuery, titleCompoundQuery, descriptionCompoundQuery)
	return b.runQuery(compound, page, resultsPerPage, wordsPerMinute)
}

func (b *BleveIndexer) runQuery(query query.Query, page, resultsPerPage int, wordsPerMinute float64) (*controller.Result, error) {
	var result controller.Result
	if page < 1 {
		page = 1
	}

	searchOptions := bleve.NewSearchRequestOptions(query, resultsPerPage, (page-1)*resultsPerPage, false)
	searchOptions.SortBy([]string{"-_score", "Series", "SeriesIndex"})
	searchOptions.Fields = []string{"Title", "Author", "Description", "Year", "Words", "Series", "SeriesIndex"}
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
			Title:       val.Fields["Title"].(string),
			Author:      val.Fields["Author"].(string),
			Description: template.HTML(val.Fields["Description"].(string)),
			Year:        val.Fields["Year"].(string),
			Words:       val.Fields["Words"].(float64),
			Series:      val.Fields["Series"].(string),
			SeriesIndex: val.Fields["SeriesIndex"].(float64),
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
