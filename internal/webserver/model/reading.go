package model

import (
	"math"
	"time"
)

// ClampReadingFraction clamps a numeric fraction to [0, 1]; NaN becomes 0.
func ClampReadingFraction(f float64) float64 {
	if math.IsNaN(f) || f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

type Reading struct {
	CreatedAt   time.Time  `gorm:"autoCreateTime"`
	UpdatedAt   time.Time  `gorm:"autoUpdateTime"`
	UserID      int        `gorm:"primaryKey"`
	Slug        string     `gorm:"primaryKey"`
	Position    string     `gorm:"type:text"`
	Fraction    *float64   `gorm:"default:null"` // 0–1 book position when set (stored as fraction, not integer percent)
	CompletedOn *time.Time `gorm:"default:null"`
}

// CompletedYearStats holds aggregated stats for documents completed in a year (or all time when Year is 0).
type CompletedYearStats struct {
	Year          int
	DocumentCount int
	ReadingTime   string // estimated reading time (e.g. "2h 30m") from word count and user's words-per-minute
}
