package model

import (
	"time"

	"github.com/svera/coreander/v4/internal/index"
)

type Highlight struct {
	CreatedAt       time.Time
	UpdatedAt       time.Time
	UserID          int    `gorm:"primaryKey;index;not null"`
	Path            string `gorm:"primaryKey;index;not null"`
	SharedByID      *int   `gorm:"index"`
	Comment         string `gorm:"type:text"`
	SharedByName    string `gorm:"-"`
	SharedByUsername string `gorm:"-"`
	Document        index.Document `gorm:"-"` // Embedded document for template access
}
