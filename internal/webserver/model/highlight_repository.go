package model

import (
	"errors"
	"log"

	"github.com/svera/coreander/v4/internal/result"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type HighlightRepository struct {
	DB        *gorm.DB
	Idx idxReader
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
func (u *HighlightRepository) Highlights(userID int, page int, resultsPerPage int, sortBy, filter string) (result.Paginated[[]AugmentedDocument], error) {
	if u.Idx == nil {
		return result.Paginated[[]AugmentedDocument]{}, errors.New("highlight repository: idx required for Highlights")
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

	if len(highlights) == 0 {
		return result.NewPaginated(resultsPerPage, page, int(total), []AugmentedDocument{}), nil
	}

	slugs := make([]string, len(highlights))
	for i, hl := range highlights {
		slugs[i] = hl.Slug
	}
	docBySlug, err := u.Idx.Documents(slugs)
	if err != nil {
		log.Printf("error getting documents for highlights: %s\n", err)
		return result.Paginated[[]AugmentedDocument]{}, err
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

	return result.NewPaginated(
		resultsPerPage,
		page,
		int(total),
		augmented,
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
