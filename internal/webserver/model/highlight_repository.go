package model

import (
	"errors"
	"log"
	"slices"
	"strings"

	"github.com/gosimple/slug"
	"github.com/svera/coreander/v5/internal/index"
	"github.com/svera/coreander/v5/internal/result"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type HighlightRepository struct {
	DB                   *gorm.DB
	Idx                  idxReader
	IllustratedMinAmount int
}

func (u *HighlightRepository) highlightListQuery(userID int, filter string) *gorm.DB {
	q := u.DB.Model(&Highlight{}).Where("user_id = ?", userID)
	switch filter {
	case "highlights":
		q = q.Where("shared_by_id IS NULL")
	case "shared":
		q = q.Where("shared_by_id IS NOT NULL")
	}
	return q
}

// Highlights returns paginated highlights as AugmentedDocuments (index-backed). Rows whose documents
// are missing from the index are omitted from Hits() but still count toward TotalHits.
//
// When searchFields has any field set, matching is done against document metadata (title, authors,
// language, subjects, publishing date, reading time, pages, illustrations), which only lives in the
// index. Since a user's highlights are a bounded, small set, all of them (for the given filter) are
// fetched from the index and paginated in memory instead of relying on SQL-level pagination.
func (u *HighlightRepository) Highlights(userID int, page int, resultsPerPage int, sortBy, filter string, searchFields index.SearchFields) (result.Paginated[[]AugmentedDocument], error) {
	if u.Idx == nil {
		return result.Paginated[[]AugmentedDocument]{}, errors.New("highlight repository: idx required for Highlights")
	}

	if hasSearchFields(searchFields) {
		return u.searchHighlights(userID, page, resultsPerPage, sortBy, filter, searchFields)
	}

	var total int64
	if err := u.highlightListQuery(userID, filter).Count(&total).Error; err != nil {
		log.Printf("error counting highlights: %s\n", err)
		return result.Paginated[[]AugmentedDocument]{}, err
	}

	highlights := []Highlight{}
	res := u.highlightListQuery(userID, filter).
		Preload("SharedBy").
		Scopes(Paginate(page, resultsPerPage)).
		Order(sortBy).
		Find(&highlights)
	if res.Error != nil {
		log.Printf("error listing highlights: %s\n", res.Error)
		return result.Paginated[[]AugmentedDocument]{}, res.Error
	}

	augmented, err := u.augmentHighlights(highlights)
	if err != nil {
		return result.Paginated[[]AugmentedDocument]{}, err
	}

	return result.NewPaginated(
		resultsPerPage,
		page,
		int(total),
		augmented,
	), nil
}

func (u *HighlightRepository) searchHighlights(userID int, page int, resultsPerPage int, sortBy, filter string, searchFields index.SearchFields) (result.Paginated[[]AugmentedDocument], error) {
	highlights := []Highlight{}
	res := u.highlightListQuery(userID, filter).
		Preload("SharedBy").
		Order(sortBy).
		Find(&highlights)
	if res.Error != nil {
		log.Printf("error listing highlights: %s\n", res.Error)
		return result.Paginated[[]AugmentedDocument]{}, res.Error
	}

	augmented, err := u.augmentHighlights(highlights)
	if err != nil {
		return result.Paginated[[]AugmentedDocument]{}, err
	}

	matching := make([]AugmentedDocument, 0, len(augmented))
	for _, doc := range augmented {
		if matchesSearchFields(doc, searchFields, u.IllustratedMinAmount) {
			matching = append(matching, doc)
		}
	}

	return result.Paginate(resultsPerPage, page, len(matching), matching), nil
}

func (u *HighlightRepository) augmentHighlights(highlights []Highlight) ([]AugmentedDocument, error) {
	if len(highlights) == 0 {
		return []AugmentedDocument{}, nil
	}

	slugs := make([]string, len(highlights))
	for i, hl := range highlights {
		slugs[i] = hl.Slug
	}
	docBySlug, err := u.Idx.Documents(slugs)
	if err != nil {
		log.Printf("error getting documents for highlights: %s\n", err)
		return nil, err
	}
	augmented := make([]AugmentedDocument, 0, len(highlights))
	for _, hl := range highlights {
		if doc, ok := docBySlug[hl.Slug]; ok {
			augmented = append(augmented, AugmentedDocument{
				Document:  doc,
				Highlight: hl,
			})
		}
	}

	return augmented, nil
}

// hasSearchFields reports whether any of the document filter fields are set, so Highlights can keep
// using the cheap SQL-only pagination path when the user isn't filtering by document metadata.
func hasSearchFields(sf index.SearchFields) bool {
	return sf.Keywords != "" ||
		sf.Language != "" ||
		sf.Subjects != "" ||
		sf.PubDateFrom != 0 ||
		sf.PubDateTo != 0 ||
		sf.EstReadTimeFrom != 0 ||
		sf.EstReadTimeTo != 0 ||
		sf.PagesFrom != 0 ||
		sf.PagesTo != 0 ||
		sf.IllustratedOnly
}

// matchesSearchFields replicates, in Go, the same matching semantics addFilters applies at the Bleve
// level for the main document search (see internal/index/bleve_document_read.go), since a user's
// highlighted documents are already fully hydrated in memory rather than queried from the index.
func matchesSearchFields(doc AugmentedDocument, sf index.SearchFields, illustratedMinAmount int) bool {
	if sf.Keywords != "" {
		query := strings.ToLower(sf.Keywords)
		matchesTitle := strings.Contains(strings.ToLower(doc.Title), query)
		matchesAuthor := false
		for _, author := range doc.Authors {
			if strings.Contains(strings.ToLower(author), query) {
				matchesAuthor = true
				break
			}
		}
		if !matchesTitle && !matchesAuthor {
			return false
		}
	}

	if sf.Language != "" && !strings.HasPrefix(doc.Language, strings.TrimSpace(sf.Language)) {
		return false
	}

	if sf.Subjects != "" {
		for _, subject := range strings.Split(sf.Subjects, ",") {
			subject = strings.TrimSpace(subject)
			if subject == "" {
				continue
			}
			if !slices.Contains(doc.SubjectsSlugs, slug.Make(subject)) {
				return false
			}
		}
	}

	if (sf.PubDateFrom != 0 || sf.PubDateTo != 0) && !matchesPubDateRange(doc, sf) {
		return false
	}

	if (sf.EstReadTimeFrom > 0 || sf.EstReadTimeTo > 0) && !matchesReadingTimeRange(doc, sf) {
		return false
	}

	if (sf.PagesFrom > 0 || sf.PagesTo > 0) && !matchesPagesRange(doc, sf) {
		return false
	}

	if sf.IllustratedOnly && illustratedMinAmount > 0 && doc.Illustrations < illustratedMinAmount {
		return false
	}

	return true
}

func matchesPubDateRange(doc AugmentedDocument, sf index.SearchFields) bool {
	if sf.PubDateFrom != 0 && doc.Publication.Date < sf.PubDateFrom {
		return false
	}
	if sf.PubDateTo != 0 && doc.Publication.Date > sf.PubDateTo {
		return false
	}
	return true
}

func matchesReadingTimeRange(doc AugmentedDocument, sf index.SearchFields) bool {
	if doc.Format == "pdf" {
		return false
	}
	fromWords := sf.EstReadTimeFrom * 60 * sf.WordsPerMinute
	toWords := sf.EstReadTimeTo * 60 * sf.WordsPerMinute
	if fromWords > 0 && doc.Words < fromWords {
		return false
	}
	if toWords > 0 && doc.Words > toWords {
		return false
	}
	return true
}

func matchesPagesRange(doc AugmentedDocument, sf index.SearchFields) bool {
	if doc.Format == "epub" {
		return false
	}
	if sf.PagesFrom > 0 && doc.Pages < sf.PagesFrom {
		return false
	}
	if sf.PagesTo > 0 && doc.Pages > sf.PagesTo {
		return false
	}
	return true
}

func (u *HighlightRepository) Total(userID int) (int, error) {
	var total int64
	res := u.DB.Table("highlights").Where("user_id = ?", userID).Count(&total)
	if res.Error != nil {
		log.Printf("error counting highlights: %s\n", res.Error)
		return 0, res.Error
	}
	return int(total), nil
}

func (u *HighlightRepository) HighlightedPaginatedResult(userID int, results result.Paginated[[]AugmentedDocument]) result.Paginated[[]AugmentedDocument] {
	highlightsBySlug := map[string]Highlight{}
	slugs := make([]string, 0, len(results.Hits()))
	searchResults := make([]AugmentedDocument, len(results.Hits()))

	for _, searchResult := range results.Hits() {
		slugs = append(slugs, searchResult.Slug)
	}
	if len(slugs) > 0 && userID > 0 {
		highlights := []Highlight{}
		res := u.DB.Model(&Highlight{}).
			Where("user_id = ? AND slug IN (?)", userID, slugs).
			Preload("SharedBy").
			Find(&highlights)
		if res.Error != nil {
			log.Printf("error listing highlight details: %s\n", res.Error)
		} else {
			for _, highlight := range highlights {
				highlightsBySlug[highlight.Slug] = highlight
			}
		}
	}

	for i, searchResult := range results.Hits() {
		highlight, ok := highlightsBySlug[searchResult.Slug]
		if !ok {
			highlight = Highlight{}
		}
		searchResults[i] = AugmentedDocument{
			Document:          searchResult.Document,
			Highlight:         highlight,
			CompletedOn:       searchResult.CompletedOn,
			ReadingPercentage: searchResult.ReadingPercentage,
		}
	}

	return result.NewPaginated(
		ResultsPerPage,
		results.Page(),
		results.TotalHits(),
		searchResults,
	)
}

func (u *HighlightRepository) Highlighted(userID int, doc AugmentedDocument) AugmentedDocument {
	var highlight Highlight
	err := u.DB.Select("user_id", "slug").
		Where("user_id = ? AND slug = ?", userID, doc.Slug).
		Take(&highlight).Error
	if err == nil {
		doc.Highlight = highlight
	}
	return doc
}

func (u *HighlightRepository) Highlight(userID int, documentSlug string) error {
	highlight := Highlight{
		UserID: userID,
		Slug:   documentSlug,
	}
	return u.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&highlight).Error
}

func (u *HighlightRepository) Remove(userID int, documentSlug string) error {
	highlight := Highlight{
		UserID: userID,
		Slug:   documentSlug,
	}
	return u.DB.Delete(&highlight).Error
}

func (u *HighlightRepository) Share(senderID int, documentSlug, comment string, recipientIDs []int) error {
	if senderID <= 0 || documentSlug == "" || len(recipientIDs) == 0 {
		return nil
	}

	shares := make([]Highlight, 0, len(recipientIDs))
	sharedByID := senderID
	for _, recipientID := range recipientIDs {
		if recipientID <= 0 {
			continue
		}
		shares = append(shares, Highlight{
			UserID:     recipientID,
			Slug:       documentSlug,
			SharedByID: &sharedByID,
			Comment:    comment,
		})
	}

	if len(shares) == 0 {
		return nil
	}

	// OnConflict{DoNothing: true} handles duplicates silently
	return u.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&shares).Error
}

func (u *HighlightRepository) RemoveDocument(documentSlug string) error {
	return u.DB.Where("slug = ?", documentSlug).Delete(&Highlight{}).Error
}
