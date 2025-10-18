package model

import (
	"log"

	"github.com/svera/coreander/v4/internal/result"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ReadingRepository struct {
	DB *gorm.DB
}

func (u *ReadingRepository) Latest(userID int, page int, resultsPerPage int) (result.Paginated[[]string], error) {
	docs := []string{}
	var total int64

	res := u.DB.Scopes(Paginate(page, resultsPerPage)).Table("readings").Select("path").Where("user_id = ?", userID).Order("updated_at DESC").Pluck("path", &docs)
	if res.Error != nil {
		log.Printf("error listing documents in progress: %s\n", res.Error)
		return result.Paginated[[]string]{}, res.Error
	}
	u.DB.Table("readings").Where("user_id = ?", userID).Count(&total)

	return result.NewPaginated(
		resultsPerPage,
		page,
		int(total),
		docs,
	), nil
}

func (u *ReadingRepository) Get(userID int, documentPath string) (Reading, error) {
	var reading Reading
	err := u.DB.Where("user_id = ? AND path = ?", userID, documentPath).First(&reading).Error
	return reading, err
}

func (u *ReadingRepository) Update(userID int, documentPath, position string) error {
	progress := Reading{
		UserID:   userID,
		Path:     documentPath,
		Position: position,
	}
	return u.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(&progress).Error
}

func (u *ReadingRepository) RemoveDocument(documentPath string) error {
	return u.DB.Where("path = ?", documentPath).Delete(&Reading{}).Error
}
