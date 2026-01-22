package model

import "gorm.io/gorm"

type ShareRepository struct {
	DB *gorm.DB
}

func (r *ShareRepository) CreateWithRecipients(share *Share, recipientIDs []int) error {
	return r.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(share).Error; err != nil {
			return err
		}
		for _, recipientID := range recipientIDs {
			shareUser := &ShareUser{
				ShareID: share.ID,
				UserID:  recipientID,
			}
			if err := tx.Create(shareUser).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
