package index

import (
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/rickb777/date/v2"
)

// SameSubjects returns an array of metadata of documents by other authors,
// which have similar subjects as the passed one and does not belong to the same collection
// They are sorted by subjects matching and date, the closest to the publishing date of the reference document first
func (b *BleveIndexer) SameSubjects(slugID string, quantity int) ([]Document, error) {
	doc, err := b.Document(slugID)
	if err != nil {
		return []Document{}, err
	}

	if len(doc.Subjects) == 0 {
		return []Document{}, err
	}

	dateLimit := float64(doc.Publication.Date)

	olderQuery := b.dateRangeSubjectsQuery(doc, nil, &dateLimit)
	olderResults, err := b.dateRangeResult(olderQuery, "-Publication.Date", quantity)
	if err != nil {
		return []Document{}, err
	}
	newerQuery := b.dateRangeSubjectsQuery(doc, &dateLimit, nil)
	newerResults, err := b.dateRangeResult(newerQuery, "Publication.Date", quantity)
	if err != nil {
		return []Document{}, err
	}

	return b.sortByTempDistance(doc.Publication.Date, newerResults, olderResults, quantity)
}

func (b *BleveIndexer) dateRangeSubjectsQuery(doc Document, minDate, maxDate *float64) *query.BooleanQuery {
	bq := bleve.NewBooleanQuery()
	subjectsCompoundQuery := bleve.NewDisjunctionQuery()

	for _, slug := range doc.SubjectsSlugs {
		qu := bleve.NewTermQuery(slug)
		qu.SetField("SubjectsSlugs")
		subjectsCompoundQuery.AddQuery(qu)
	}

	if doc.SeriesSlug != "" {
		sq := bleve.NewTermQuery(doc.SeriesSlug)
		sq.SetField("SeriesSlug")
		bq.AddMustNot(sq)
	}

	bq.AddMust(subjectsCompoundQuery)
	bq.AddMustNot(bleve.NewDocIDQuery([]string{doc.ID}))

	authorsCompoundQuery := bleve.NewDisjunctionQuery()
	for _, slug := range doc.AuthorsSlugs {
		qa := bleve.NewTermQuery(slug)
		qa.SetField("AuthorsSlugs")
		authorsCompoundQuery.AddQuery(qa)
	}
	bq.AddMustNot(authorsCompoundQuery)

	typeQuery := bleve.NewTermQuery(TypeDocument)
	typeQuery.SetField("Type")
	bq.AddMust(typeQuery)

	rangeQuery := bleve.NewNumericRangeQuery(minDate, maxDate)
	// We set the boost to 0 to avoid it being used to calculate the score
	rangeQuery.SetBoost(0)
	rangeQuery.SetField("Publication.Date")
	bq.AddMust(rangeQuery)

	return bq
}

func (b *BleveIndexer) dateRangeResult(query *query.BooleanQuery, dateSort string, quantity int) ([]*search.DocumentMatch, error) {
	var err error

	resultSet := make([]*search.DocumentMatch, 0, quantity)
	searchOptions := bleve.NewSearchRequestOptions(query, 4, 0, false)
	searchOptions.SortBy([]string{"-_score", dateSort})
	searchOptions.Fields = []string{"*"}
	current, err := b.idx.Search(searchOptions)
	if err != nil {
		return resultSet, err
	}
	if current.Total == 0 {
		return resultSet, nil
	}

	return append(resultSet, current.Hits...), nil
}

func (b *BleveIndexer) sortByTempDistance(referenceDate date.Date, newerResults, olderResults []*search.DocumentMatch, quantity int) ([]Document, error) {
	totalResults := len(newerResults) + len(olderResults)
	if totalResults < quantity {
		quantity = totalResults
	}

	docs := make([]Document, 0, quantity)

	if len(newerResults) == 0 || len(olderResults) == 0 {
		for _, doc := range olderResults {
			docs = append(docs, hydrateDocument(doc))
		}
		for _, doc := range newerResults {
			docs = append(docs, hydrateDocument(doc))
		}
		return docs, nil
	}

	newerPos, olderPos := 0, 0
	for {
		if len(docs) == quantity {
			return docs, nil
		}

		if len(newerResults) == newerPos {
			for _, doc := range olderResults[olderPos:] {
				docs = append(docs, hydrateDocument(doc))
			}
			return docs, nil
		}

		if len(olderResults) == olderPos {
			for _, doc := range newerResults[newerPos:] {
				docs = append(docs, hydrateDocument(doc))
			}
			return docs, nil
		}

		if newerResults[newerPos].Score > olderResults[olderPos].Score {
			docs = append(docs, hydrateDocument(newerResults[newerPos]))
			newerPos++
			continue
		}
		if olderResults[olderPos].Score > newerResults[newerPos].Score {
			docs = append(docs, hydrateDocument(olderResults[olderPos]))
			olderPos++
			continue
		}

		newerDoc := hydrateDocument(newerResults[newerPos])
		newerDocTempDistance := newerDoc.Publication.Date - referenceDate
		if newerDocTempDistance < 0 {
			newerDocTempDistance = -newerDocTempDistance
		}

		OlderDoc := hydrateDocument(olderResults[olderPos])
		olderDocTempDistance := OlderDoc.Publication.Date - referenceDate
		if olderDocTempDistance < 0 {
			olderDocTempDistance = -olderDocTempDistance
		}

		if newerDocTempDistance < olderDocTempDistance {
			docs = append(docs, newerDoc)
			newerPos++
		} else {
			docs = append(docs, OlderDoc)
			olderPos++
		}
	}
}

// SameAuthors returns an array of metadata of documents by the same authors which
// does not belong to the same collection
func (b *BleveIndexer) SameAuthors(slugID string, quantity int) ([]Document, error) {
	doc, err := b.Document(slugID)
	if err != nil {
		return []Document{}, err
	}

	if len(doc.Authors) == 0 {
		return []Document{}, err
	}

	authorsCompoundQuery := bleve.NewDisjunctionQuery()
	for _, slug := range doc.AuthorsSlugs {
		qu := bleve.NewTermQuery(slug)
		qu.SetField("AuthorsSlugs")
		authorsCompoundQuery.AddQuery(qu)
	}
	bq := bleve.NewBooleanQuery()
	bq.AddMust(authorsCompoundQuery)
	bq.AddMustNot(bleve.NewDocIDQuery([]string{doc.ID}))

	if doc.Series != "" {
		sq := bleve.NewTermQuery(doc.SeriesSlug)
		sq.SetField("SeriesSlug")
		bq.AddMustNot(sq)
	}

	typeQuery := bleve.NewTermQuery(TypeDocument)
	typeQuery.SetField("Type")
	bq.AddMust(typeQuery)

	return b.runQuery(bq, quantity, []string{"-_score", "Series", "SeriesIndex"})
}

// SameSeries returns an array of metadata of documents in the same series
func (b *BleveIndexer) SameSeries(slugID string, quantity int) ([]Document, error) {
	doc, err := b.Document(slugID)
	if err != nil {
		return []Document{}, err
	}

	if doc.Series == "" {
		return []Document{}, err
	}

	bq := bleve.NewBooleanQuery()
	bq.AddMustNot(bleve.NewDocIDQuery([]string{doc.ID}))

	sq := bleve.NewTermQuery(doc.SeriesSlug)
	sq.SetField("SeriesSlug")
	bq.AddMust(sq)

	typeQuery := bleve.NewTermQuery(TypeDocument)
	typeQuery.SetField("Type")
	bq.AddMust(typeQuery)

	return b.runQuery(bq, quantity, []string{"-_score", "Series", "SeriesIndex"})
}
