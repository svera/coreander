package index

import (
	"math"
	"sort"

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
		return []Document{}, nil
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

	return b.sortByTempDistance(float64(doc.Publication.Date), append(olderResults, newerResults...), quantity)
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

func (b *BleveIndexer) dateRangeResult(query *query.BooleanQuery, dateSort string, quantity int) (search.DocumentMatchCollection, error) {
	searchOptions := bleve.NewSearchRequestOptions(query, quantity, 0, false)
	searchOptions.SortBy([]string{"-_score", dateSort})
	searchOptions.Fields = []string{"*"}
	result, err := b.idx.Search(searchOptions)
	if err != nil {
		return nil, err
	}

	return result.Hits, nil
}

func (b *BleveIndexer) sortByTempDistance(referenceDate float64, results search.DocumentMatchCollection, quantity int) ([]Document, error) {
	if len(results) < quantity {
		quantity = len(results)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}

		return distanceToDate(referenceDate, results[i]) < distanceToDate(referenceDate, results[j])
	})

	docs := make([]Document, 0, quantity)

	for i := range quantity {
		docs = append(docs, hydrateDocument(results[i]))
	}

	return docs, nil
}

func distanceToDate(referenceDate float64, match *search.DocumentMatch) float64 {
	var date float64

	if match.Fields["Publication.Date"] != nil {
		date = match.Fields["Publication.Date"].(float64)
	}
	return math.Abs(date - referenceDate)
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
