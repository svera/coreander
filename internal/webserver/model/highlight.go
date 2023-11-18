package model

import (
	"time"
)

type Highlight struct {
	CreatedAt time.Time
	UpdatedAt time.Time
	UserID    int    `gorm:"primaryKey"`
	Path      string `gorm:"primaryKey"`
}
