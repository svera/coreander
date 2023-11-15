package model

import (
	"log"

	"github.com/svera/coreander/v3/internal/metadata"
	"github.com/svera/coreander/v3/internal/search"
	"golang.org/x/exp/slices"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type HighlightRepository struct {
	DB *gorm.DB
}

func (u *HighlightRepository) Highlights(userID int, page int, resultsPerPage int) (search.PaginatedResult[[]metadata.Document], error) {
	highlights := []string{}
	var total int64

	result := u.DB.Scopes(Paginate(page, resultsPerPage)).Table("highlights").Select("path").Where("user_id = ?", userID).Order("created_at DESC").Pluck("path", &highlights)
	if result.Error != nil {
		log.Printf("error listing highlights: %s\n", result.Error)
	}
	u.DB.Table("highlights").Where("user_id = ?", userID).Count(&total)

	docs := make([]metadata.Document, len(highlights))

	for i, path := range highlights {
		docs[i].ID = path
	}

	return search.NewPaginatedResult[[]metadata.Document](
		resultsPerPage,
		page,
		int(total),
		docs,
	), result.Error
}

func (u *HighlightRepository) HighlightedPaginatedResult(userID int, results search.PaginatedResult[[]metadata.Document]) search.PaginatedResult[[]metadata.Document] {
	highlights := make([]string, 0, len(results.Hits()))
	paths := make([]string, 0, len(results.Hits()))
	documents := make([]metadata.Document, len(results.Hits()))

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

	return search.NewPaginatedResult[[]metadata.Document](
		ResultsPerPage,
		results.Page(),
		results.TotalHits(),
		documents,
	)
}

func (u *HighlightRepository) Highlighted(userID int, doc metadata.Document) metadata.Document {
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
