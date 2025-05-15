package index

import (
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search"
	"github.com/blevesearch/bleve/v2/search/query"
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

	bqNewer := bleve.NewBooleanQuery()
	bqOlder := bleve.NewBooleanQuery()
	subjectsCompoundQuery := bleve.NewDisjunctionQuery()

	for _, slug := range doc.SubjectsSlugs {
		qu := bleve.NewTermQuery(slug)
		qu.SetField("SubjectsSlugs")
		subjectsCompoundQuery.AddQuery(qu)
	}

	if doc.SeriesSlug != "" {
		sq := bleve.NewTermQuery(doc.SeriesSlug)
		sq.SetField("SeriesSlug")
		bqNewer.AddMustNot(sq)
		bqOlder.AddMustNot(sq)
	}

	bqNewer.AddMust(subjectsCompoundQuery)
	bqNewer.AddMustNot(bleve.NewDocIDQuery([]string{doc.ID}))
	bqOlder.AddMust(subjectsCompoundQuery)
	bqOlder.AddMustNot(bleve.NewDocIDQuery([]string{doc.ID}))

	authorsCompoundQuery := bleve.NewDisjunctionQuery()
	for _, slug := range doc.AuthorsSlugs {
		qa := bleve.NewTermQuery(slug)
		qa.SetField("AuthorsSlugs")
		authorsCompoundQuery.AddQuery(qa)
	}
	bqNewer.AddMustNot(authorsCompoundQuery)
	bqOlder.AddMustNot(authorsCompoundQuery)

	typeQuery := bleve.NewTermQuery(TypeDocument)
	typeQuery.SetField("Type")
	bqNewer.AddMust(typeQuery)
	bqOlder.AddMust(typeQuery)

	dateLimit := float64(doc.Publication.Date)

	olderDocsQuery := bleve.NewNumericRangeQuery(nil, &dateLimit)
	// we don't want to include date in the score calculation
	olderDocsQuery.SetBoost(0)
	olderDocsQuery.SetField("Publication.Date")
	olderResults, err := b.dateRangeResult(bqOlder, olderDocsQuery, "-Publication.Date", quantity)
	if err != nil {
		return []Document{}, err
	}

	newerDocsQuery := bleve.NewNumericRangeQuery(&dateLimit, nil)
	newerDocsQuery.SetBoost(0)
	newerDocsQuery.SetField("Publication.Date")
	newerResults, err := b.dateRangeResult(bqNewer, newerDocsQuery, "Publication.Date", quantity)
	if err != nil {
		return []Document{}, err
	}

	return b.sortByTempDistance(doc, newerResults, olderResults, quantity)
}

func (b *BleveIndexer) dateRangeResult(query *query.BooleanQuery, rangeQuery *query.NumericRangeQuery, dateSort string, quantity int) ([]*search.DocumentMatch, error) {
	var err error
	query.AddMust(rangeQuery)

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

func (b *BleveIndexer) sortByTempDistance(referenceDoc Document, newerResults, olderResults []*search.DocumentMatch, quantity int) ([]Document, error) {
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
		newerDocTempDistance := newerDoc.Publication.Date - referenceDoc.Publication.Date
		if newerDocTempDistance < 0 {
			newerDocTempDistance = -newerDocTempDistance
		}

		OlderDoc := hydrateDocument(olderResults[olderPos])
		olderDocTempDistance := OlderDoc.Publication.Date - referenceDoc.Publication.Date
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
