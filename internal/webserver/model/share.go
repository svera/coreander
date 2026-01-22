package model

import "time"

type Share struct {
	ID           uint      `gorm:"primarykey"`
	CreatedAt    time.Time `gorm:"autoCreateTime"`
	UserID       *int      `gorm:"index"`
	User         User      `gorm:"constraint:OnDelete:CASCADE;"`
	DocumentSlug *string   `gorm:"index"`
	Comment      string    `gorm:"type:text"`
}
