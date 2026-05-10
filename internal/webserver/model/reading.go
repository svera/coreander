package model

import (
	"time"
)

// ClampReadingPercentage clamps v to [0, 100].
func ClampReadingPercentage(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

type Reading struct {
	CreatedAt   time.Time  `gorm:"autoCreateTime"`
	UpdatedAt   time.Time  `gorm:"autoUpdateTime"`
	UserID      int        `gorm:"primaryKey"`
	Slug        string     `gorm:"primaryKey"`
	Position    string     `gorm:"type:text"`
	Percentage  int        `gorm:"default:0"` // 0–100; unset is stored as 0
	CompletedOn *time.Time `gorm:"default:null"`
}

// CompletedYearStats holds aggregated stats for documents completed in a year (or all time when Year is 0).
type CompletedYearStats struct {
	Year          int
	DocumentCount int
	ReadingTime   string // estimated reading time (e.g. "2h 30m") from word count and user's words-per-minute
}
