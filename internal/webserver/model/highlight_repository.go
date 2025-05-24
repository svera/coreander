package model

import (
	"log"

	"github.com/svera/coreander/v4/internal/index"
	"github.com/svera/coreander/v4/internal/result"
	"golang.org/x/exp/slices"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type HighlightRepository struct {
	DB *gorm.DB
}

func (u *HighlightRepository) Highlights(userID int, page int, resultsPerPage int) (result.Paginated[[]string], error) {
	highlights := []string{}
	var total int64

	res := u.DB.Scopes(Paginate(page, resultsPerPage)).Table("highlights").Select("path").Where("user_id = ?", userID).Order("created_at DESC").Pluck("path", &highlights)
	if res.Error != nil {
		log.Printf("error listing highlights: %s\n", res.Error)
		return result.Paginated[[]string]{}, res.Error
	}
	u.DB.Table("highlights").Where("user_id = ?", userID).Count(&total)

	return result.NewPaginated(
		resultsPerPage,
		page,
		int(total),
		highlights,
	), nil
}

func (u *HighlightRepository) HighlightedPaginatedResult(userID int, results result.Paginated[[]index.Document]) result.Paginated[[]index.Document] {
	highlights := make([]string, 0, len(results.Hits()))
	paths := make([]string, 0, len(results.Hits()))
	documents := make([]index.Document, len(results.Hits()))

	for _, path := range results.Hits() {
		paths = append(paths, path.ID)
	}
	u.DB.Table("highlights").Where(
		"user_id = ? AND path IN (?)",
		userID,
		paths,
	).Pluck("path", &highlights)

	for i, doc := range results.Hits() {
		documents[i] = doc
		documents[i].Highlighted = slices.Contains(highlights, doc.ID)
	}

	return result.NewPaginated(
		ResultsPerPage,
		results.Page(),
		results.TotalHits(),
		documents,
	)
}

func (u *HighlightRepository) Highlighted(userID int, doc index.Document) index.Document {
	var count int64

	u.DB.Table("highlights").Where(
		"user_id = ? AND path = ?",
		userID,
		doc.ID,
	).Count(&count)

	if count == 1 {
		doc.Highlighted = true
	}
	return doc
}

func (u *HighlightRepository) Highlight(userID int, documentPath string) error {
	highlight := Highlight{
		UserID: userID,
		Path:   documentPath,
	}
	return u.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&highlight).Error
}

func (u *HighlightRepository) Remove(userID int, documentPath string) error {
	highlight := Highlight{
		UserID: userID,
		Path:   documentPath,
	}
	return u.DB.Delete(&highlight).Error
}

func (u *HighlightRepository) RemoveDocument(documentPath string) error {
	return u.DB.Where("path = ?", documentPath).Delete(&Highlight{}).Error
}
