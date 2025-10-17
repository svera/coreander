package model

import (
	"time"
)

type Reading struct {
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
	UserID    int       `gorm:"primaryKey"`
	Path      string    `gorm:"primaryKey"`
	CFI       string    `gorm:"type:text"`
}
