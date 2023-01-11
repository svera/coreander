package index

import (
	"fmt"
	"html/template"
	"math"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/svera/coreander/internal/metadata"
	"github.com/svera/coreander/internal/webserver"
)

// Search look for documents which match with the passed keywords. Returns a maximum <resultsPerPage> books, offset by <page>
func (b *BleveIndexer) Search(keywords string, page, resultsPerPage int) (*webserver.Result, error) {
	query := bleve.NewQueryStringQuery(keywords)

	return b.runQuery(query, page, resultsPerPage)
}

func (b *BleveIndexer) runQuery(query query.Query, page, resultsPerPage int) (*webserver.Result, error) {
	var result webserver.Result
	if page < 1 {
		page = 1
	}

	searchOptions := bleve.NewSearchRequestOptions(query, resultsPerPage, (page-1)*resultsPerPage, false)
	searchOptions.SortBy([]string{"Series", "SeriesIndex"})
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
	result = webserver.Result{
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
