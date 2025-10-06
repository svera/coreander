package model

import (
	"log"

	"github.com/svera/coreander/v4/internal/result"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type HistoryRepository struct {
	DB *gorm.DB
}

func (u *HistoryRepository) LatestReads(userID int, page int, resultsPerPage int) (result.Paginated[[]string], error) {
	docs := []string{}
	var total int64

	res := u.DB.Scopes(Paginate(page, resultsPerPage)).Table("history").Select("path").Where("user_id = ? AND action = ?", userID, HistoryActionRead).Order("updated_at DESC").Pluck("path", &docs)
	if res.Error != nil {
		log.Printf("error listing documents in progress: %s\n", res.Error)
		return result.Paginated[[]string]{}, res.Error
	}
	u.DB.Table("history").Where("user_id = ?", userID).Count(&total)

	return result.NewPaginated(
		resultsPerPage,
		page,
		int(total),
		docs,
	), nil
}

func (u *HistoryRepository) UpdateReading(userID int, documentPath string) error {
	progress := History{
		UserID: userID,
		Path:   documentPath,
		Action: HistoryActionRead,
	}
	return u.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(&progress).Error
}

func (u *HistoryRepository) Remove(documentPath string) error {
	return u.DB.Where("path = ?", documentPath).Delete(&History{}).Error
}
