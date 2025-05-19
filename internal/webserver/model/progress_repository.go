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

func (u *ReadingRepository) List(userID int, page int, resultsPerPage int) (result.Paginated[[]string], error) {
	docs := []string{}
	var total int64

	res := u.DB.Scopes(Paginate(page, resultsPerPage)).Table("progress").Select("path").Where("user_id = ?", userID).Order("updated_at DESC").Pluck("path", &docs)
	if res.Error != nil {
		log.Printf("error listing documents in progress: %s\n", res.Error)
	}
	u.DB.Table("progress").Where("user_id = ?", userID).Count(&total)

	return result.NewPaginated[[]string](
		resultsPerPage,
		page,
		int(total),
		docs,
	), res.Error
}

func (u *ReadingRepository) Update(userID int, documentPath string) error {
	progress := Progress{
		UserID: userID,
		Path:   documentPath,
	}
	return u.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(&progress).Error
}
