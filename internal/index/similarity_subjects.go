package index

import (
	"math"
	"slices"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search/query"
)

const similarityMaxSubjectFrequency = 0.20

func (s *similarityPhraseStatistics) prepareSubjectFrequencies() {
	threshold := math.Max(float64(DefaultMinCommonTextRankAbsoluteCount), float64(s.DocumentCount)*similarityMaxSubjectFrequency)
	s.commonSubjects = nil
	for subject, count := range s.SubjectCounts {
		if subject != "" && float64(count) > threshold {
			s.commonSubjects = append(s.commonSubjects, subject)
		}
	}
	slices.Sort(s.commonSubjects)
}

func usefulSimilaritySubjects(subjects, commonSubjects []string) []string {
	var useful []string
	for _, subject := range subjects {
		_, common := slices.BinarySearch(commonSubjects, subject)
		if subject != "" && !common && !slices.Contains(useful, subject) {
			useful = append(useful, subject)
		}
	}
	return useful
}

// Match any subject outside the frequent labels using the existing keyword
// index. Separate ranges matter: excluding documents containing a generic
// label would incorrectly reject books with both common and useful subjects.
func usefulSimilaritySubjectsQuery(commonSubjects []string) query.Query {
	useful := bleve.NewDisjunctionQuery()
	exclusive := false
	lower := "\x00"
	for _, upper := range commonSubjects {
		q := bleve.NewTermRangeInclusiveQuery(lower, upper, &exclusive, &exclusive)
		q.SetField("SubjectsSlugs")
		useful.AddQuery(q)
		lower = upper
	}
	q := bleve.NewTermRangeInclusiveQuery(lower, "", &exclusive, &exclusive)
	q.SetField("SubjectsSlugs")
	useful.AddQuery(q)
	return useful
}

func addSimilaritySubjectFilter(reference Document, candidates *query.BooleanQuery, stats *similarityPhraseStatistics) {
	subjects := usefulSimilaritySubjects(reference.SubjectsSlugs, stats.commonSubjects)
	if len(subjects) == 0 {
		return
	}
	shared := bleve.NewDisjunctionQuery()
	for _, subject := range subjects {
		q := bleve.NewTermQuery(subject)
		q.SetField("SubjectsSlugs")
		shared.AddQuery(q)
	}
	noUsefulSubjects := bleve.NewBooleanQuery()
	noUsefulSubjects.AddMust(bleve.NewMatchAllQuery())
	noUsefulSubjects.AddMustNot(usefulSimilaritySubjectsQuery(stats.commonSubjects))
	candidates.AddFilter(bleve.NewDisjunctionQuery(shared, noUsefulSubjects))
}
