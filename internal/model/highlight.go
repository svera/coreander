package model

import (
	"time"

	"gorm.io/gorm"
)

type Highlight struct {
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
	UserID    int            `gorm:"primaryKey"`
	Path      string         `gorm:"primaryKey"`
}
