package model

import (
	"log"

	"github.com/svera/coreander/v4/internal/result"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type HighlightRepository struct {
	DB *gorm.DB
}

func (u *HighlightRepository) Highlights(userID int, page int, resultsPerPage int, sortBy, filter string) (result.Paginated[[]Highlight], error) {
	var total int64

	query := u.DB.Model(&Highlight{}).
		Where("user_id = ?", userID)

	switch filter {
	case "highlights":
		query = query.Where("shared_by_id IS NULL")
	case "shared":
		query = query.Where("shared_by_id IS NOT NULL")
	}

	countQuery := query
	countQuery.Count(&total)

	highlights := []Highlight{}
	res := query.Preload("SharedBy").Scopes(Paginate(page, resultsPerPage)).Order(sortBy).Find(&highlights)
	if res.Error != nil {
		log.Printf("error listing highlights: %s\n", res.Error)
		return result.Paginated[[]Highlight]{}, res.Error
	}

	return result.NewPaginated(
		resultsPerPage,
		page,
		int(total),
		highlights,
	), nil
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
			Document:    searchResult.Document,
			Highlight:   highlight,
			CompletedOn: searchResult.CompletedOn,
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
	var count int64

	u.DB.Table("highlights").Where(
		"user_id = ? AND slug = ?",
		userID,
		doc.Slug,
	).Count(&count)

	if count == 1 {
		doc.Highlight = Highlight{
			UserID: userID,
			Slug:   doc.Slug,
		}
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

func (u *HighlightRepository) Share(senderID int, documentID, documentSlug, comment string, recipientIDs []int) error {
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
