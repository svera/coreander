package model

import (
	"time"
)

type Reading struct {
	CreatedAt   time.Time  `gorm:"autoCreateTime"`
	UpdatedAt   time.Time  `gorm:"autoUpdateTime"`
	UserID      int        `gorm:"primaryKey"`
	Slug        string     `gorm:"primaryKey"`
	Position    string     `gorm:"type:text"`
	CompletedOn *time.Time `gorm:"default:null"`
}
