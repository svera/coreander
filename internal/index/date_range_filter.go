package index

import (
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/rickb777/date/v2"
)

func addDateRangeFilter(filtersQuery *query.ConjunctionQuery, field string, from, to date.Date) {
	if from == 0 && to == 0 {
		return
	}
	minDate := float64(from)
	q := bleve.NewNumericRangeQuery(nil, nil)
	if from != 0 {
		q.Min = &minDate
	}
	if to != 0 {
		maxDate := float64(to) + 1
		q.Max = &maxDate
	}
	q.SetField(field)
	filtersQuery.AddQuery(q)
}
