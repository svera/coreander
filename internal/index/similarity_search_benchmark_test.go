package index

import (
	"fmt"
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search/query"
)

func BenchmarkSimilaritySubjectQuery(b *testing.B) {
	for _, count := range []int{100, 1000, 5000} {
		documents, err := bleve.NewMemOnly(CreateDocumentsMapping())
		if err != nil {
			b.Fatal(err)
		}
		subjects := make([]string, count)
		batch := documents.NewBatch()
		for i := range count {
			subjects[i] = fmt.Sprintf("topic-%05d", i)
			doc := evidenceDocument(subjects[i])
			doc.SubjectsSlugs = []string{"common", subjects[i]}
			if err := batch.Index(doc.ID, doc); err != nil {
				b.Fatal(err)
			}
		}
		if err := documents.Batch(batch); err != nil {
			b.Fatal(err)
		}
		cached := usefulSimilaritySubjectsQuery(subjects)
		for _, mode := range []string{"cached-explicit", "previous-ranges"} {
			b.Run(fmt.Sprintf("%d/%s", count, mode), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					var q query.Query
					if mode == "cached-explicit" {
						q = cached
					} else {
						exclusive := false
						before := bleve.NewTermRangeInclusiveQuery("\x00", "common", &exclusive, &exclusive)
						after := bleve.NewTermRangeInclusiveQuery("common", "", &exclusive, &exclusive)
						before.SetField("SubjectsSlugs")
						after.SetField("SubjectsSlugs")
						q = bleve.NewDisjunctionQuery(before, after)
					}
					matches, err := documents.Search(bleve.NewSearchRequestOptions(q, 0, 0, false))
					if err != nil {
						b.Fatal(err)
					}
					if matches.Total != uint64(count) {
						b.Fatalf("matched %d documents, want %d", matches.Total, count)
					}
				}
			})
		}
		if err := documents.Close(); err != nil {
			b.Fatal(err)
		}
	}
}
