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
	Document        index.Document `gorm:"-"` // Embedded document for template access
	SharedBy        *User          `gorm:"foreignKey:SharedByID"`
}

type SearchResult struct {
	Document index.Document
	Highlight Highlight
}
