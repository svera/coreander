package model

import (
	"time"
)

type Reading struct {
	CreatedAt   time.Time  `gorm:"autoCreateTime"`
	UpdatedAt   time.Time  `gorm:"autoUpdateTime"`
	UserID      int        `gorm:"primaryKey"`
	Path        string     `gorm:"primaryKey"`
	Position    string     `gorm:"type:text"`
	Completed   bool       `gorm:"default:false"`
	CompletedAt *time.Time `gorm:"default:null"`
}
