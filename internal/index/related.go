package index

import (
	"fmt"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search"
	"github.com/blevesearch/bleve/v2/search/query"
)

// SameSubjects returns an array of metadata of documents by other authors, different between each other,
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
	// we don't want to include date as part of the score
	olderDocsQuery.SetBoost(0)
	olderDocsQuery.SetField("Publication.Date")
	olderResults, err := b.dateRangeResult(bqOlder, olderDocsQuery, authorsCompoundQuery, "-Publication.Date", quantity)
	if err != nil {
		return []Document{}, err
	}

	newerDocsQuery := bleve.NewNumericRangeQuery(&dateLimit, nil)
	newerDocsQuery.SetBoost(0)
	newerDocsQuery.SetField("Publication.Date")
	newerResults, err := b.dateRangeResult(bqNewer, newerDocsQuery, authorsCompoundQuery, "Publication.Date", quantity)
	if err != nil {
		return []Document{}, err
	}

	fmt.Println("newer")
	for i := range newerResults {
		fmt.Println(newerResults[i].Fields["Title"].(string), newerResults[i].Score)
	}
	fmt.Println("older")
	for i := range olderResults {
		fmt.Println(olderResults[i].Fields["Title"].(string), olderResults[i].Score)
	}
	return b.sortByTempDistance(doc, newerResults, olderResults, quantity)
}

func (b *BleveIndexer) dateRangeResult(query *query.BooleanQuery, rangeQuery *query.NumericRangeQuery, authorsCompoundQuery *query.DisjunctionQuery, dateSort string, quantity int) ([]*search.DocumentMatch, error) {
	var err error
	query.AddMust(rangeQuery)

	resultSet := make([]*search.DocumentMatch, 0, quantity)
	//for range quantity {
	searchOptions := bleve.NewSearchRequestOptions(query, 4, 0, false)
	searchOptions.SortBy([]string{"-_score", dateSort})
	searchOptions.Fields = []string{"*"}
	current, err := b.idx.Search(searchOptions)

	//current, err := b.runQuery(query, 1, []string{"-_score", dateSort})
	if err != nil {
		return resultSet, err
	}
	if current.Total == 0 {
		return resultSet, nil
	}

	resultSet = append(resultSet, current.Hits...)
	/*
		for _, slug := range slicer(current.Hits[0].Fields["AuthorsSlugs"]) {
			qa := bleve.NewTermQuery(slug)
			qa.SetField("AuthorsSlugs")
			authorsCompoundQuery.AddQuery(qa)
		}
		query.AddMustNot(authorsCompoundQuery)
	*/
	//}

	return resultSet, err
}

func (b *BleveIndexer) sortByTempDistance(referenceDoc Document, topResults, bottomResults []*search.DocumentMatch, quantity int) ([]Document, error) {
	totalResults := len(topResults) + len(bottomResults)
	if totalResults < quantity {
		quantity = totalResults
	}

	docs := make([]Document, 0, quantity)

	if len(topResults) == 0 || len(bottomResults) == 0 {
		for _, doc := range bottomResults {
			docs = append(docs, hydrateDocument(doc))
		}
		for _, doc := range topResults {
			docs = append(docs, hydrateDocument(doc))
		}
		return docs, nil
	}

	topPos, bottomPos := 0, 0
	for {
		if len(docs) == quantity {
			return docs, nil
		}
		if topResults[topPos].Score > bottomResults[bottomPos].Score {
			docs = append(docs, hydrateDocument(topResults[topPos]))
			topPos++
			continue
		}
		if bottomResults[bottomPos].Score > topResults[topPos].Score {
			docs = append(docs, hydrateDocument(bottomResults[bottomPos]))
			bottomPos++
			continue
		}

		topDoc := hydrateDocument(topResults[topPos])
		bottomDoc := hydrateDocument(bottomResults[bottomPos])

		topDocTempDistance := topDoc.Publication.Date.Year() - referenceDoc.Publication.Date.Year()
		if topDocTempDistance < 0 {
			topDocTempDistance = -topDocTempDistance
		}
		bottomDocTempDistance := bottomDoc.Publication.Date.Year() - referenceDoc.Publication.Date.Year()
		if bottomDocTempDistance < 0 {
			bottomDocTempDistance = -bottomDocTempDistance
		}
		if topDocTempDistance < bottomDocTempDistance {
			docs = append(docs, topDoc)
			topPos++
		} else {
			docs = append(docs, bottomDoc)
			bottomPos++
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
