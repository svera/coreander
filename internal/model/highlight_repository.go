package model

import (
	"log"

	"github.com/svera/coreander/v3/internal/search"
	"golang.org/x/exp/slices"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type HighlightRepository struct {
	DB *gorm.DB
}

func (u *HighlightRepository) Highlights(userID int, page int, resultsPerPage int) (search.PaginatedResult, error) {
	highlights := []string{}
	var total int64

	result := u.DB.Scopes(Paginate(page, resultsPerPage)).Table("highlights").Select("path").Where("user_id = ?", userID).Order("created_at DESC").Pluck("path", &highlights)
	if result.Error != nil {
		log.Printf("error listing highlights: %s\n", result.Error)
	}
	u.DB.Table("highlights").Where("user_id = ?", userID).Count(&total)

	paginatedResult := search.PaginatedResult{
		Page:       page,
		Hits:       make([]search.Document, len(highlights)),
		TotalHits:  int(total),
		TotalPages: search.CalculateTotalPages(uint64(total), ResultsPerPage),
	}

	for i, path := range highlights {
		paginatedResult.Hits[i].ID = path
	}

	return paginatedResult, result.Error
}

func (u *HighlightRepository) Highlighted(userID int, documents []search.Document) []search.Document {
	highlights := make([]string, 0, len(documents))
	paths := make([]string, 0, len(documents))
	for _, path := range documents {
		paths = append(paths, path.ID)
	}
	u.DB.Table("highlights").Where(
		"user_id = ? AND path IN (?)",
		userID,
		paths,
	).Pluck("path", &highlights)
	for i, doc := range documents {
		documents[i].Highlighted = slices.Contains(highlights, doc.ID)
	}

	return documents
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
