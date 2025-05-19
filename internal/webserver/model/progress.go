package model

import (
	"time"
)

type Progress struct {
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
	UserID    int       `gorm:"primaryKey"`
	Path      string    `gorm:"primaryKey"`
}

func (Progress) TableName() string {
	return "progress"
}
