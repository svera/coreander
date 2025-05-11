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

	bqTop := bleve.NewBooleanQuery()
	bqBottom := bleve.NewBooleanQuery()
	subjectsCompoundQuery := bleve.NewDisjunctionQuery()

	for _, slug := range doc.SubjectsSlugs {
		qu := bleve.NewTermQuery(slug)
		qu.SetField("SubjectsSlugs")
		subjectsCompoundQuery.AddQuery(qu)
	}

	if doc.SeriesSlug != "" {
		sq := bleve.NewTermQuery(doc.SeriesSlug)
		sq.SetField("SeriesSlug")
		bqTop.AddMustNot(sq)
		bqBottom.AddMustNot(sq)
	}

	bqTop.AddMust(subjectsCompoundQuery)
	bqTop.AddMustNot(bleve.NewDocIDQuery([]string{doc.ID}))
	bqBottom.AddMust(subjectsCompoundQuery)
	bqBottom.AddMustNot(bleve.NewDocIDQuery([]string{doc.ID}))

	authorsCompoundQuery := bleve.NewDisjunctionQuery()
	for _, slug := range doc.AuthorsSlugs {
		qa := bleve.NewTermQuery(slug)
		qa.SetField("AuthorsSlugs")
		authorsCompoundQuery.AddQuery(qa)
	}
	bqTop.AddMustNot(authorsCompoundQuery)
	bqBottom.AddMustNot(authorsCompoundQuery)

	typeQuery := bleve.NewTermQuery(TypeDocument)
	typeQuery.SetField("Type")
	bqTop.AddMust(typeQuery)
	bqBottom.AddMust(typeQuery)

	dateLimit := float64(doc.Publication.Date)

	olderDocsQuery := bleve.NewNumericRangeQuery(nil, &dateLimit)
	olderDocsQuery.SetField("Publication.Date")
	bottomResults, err := b.dateRangeResult(bqBottom, olderDocsQuery, authorsCompoundQuery, "-Publication.Date", quantity)
	if err != nil {
		return []Document{}, err
	}

	newerDocsQuery := bleve.NewNumericRangeQuery(&dateLimit, nil)
	newerDocsQuery.SetField("Publication.Date")
	topResults, err := b.dateRangeResult(bqTop, newerDocsQuery, authorsCompoundQuery, "Publication.Date", quantity)

	if err != nil {
		return []Document{}, err
	}

	fmt.Println("top")
	for i := range topResults {
		fmt.Println(topResults[i].Fields["Title"].(string), topResults[i].Score)
	}
	fmt.Println("bottom")
	for i := range bottomResults {
		fmt.Println(bottomResults[i].Fields["Title"].(string), bottomResults[i].Score)
	}
	return b.sortByTempDistance(doc, topResults, bottomResults, quantity)
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
	docs := make([]Document, 0, quantity)
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
