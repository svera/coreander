package model

import (
	"time"
)

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
	UpdatedAt   time.Time  `gorm:"autoUpdateTime;index:idx_readings_user_in_progress,priority:2"`
	UserID      int        `gorm:"primaryKey;index:idx_readings_user_completed,priority:1;index:idx_readings_user_in_progress,priority:1"`
	Slug        string     `gorm:"primaryKey;index:idx_readings_slug"`
	Position    string     `gorm:"type:text"`
	Percentage  int        `gorm:"default:0"`
	CompletedOn *time.Time `gorm:"default:null;index:idx_readings_user_completed,priority:2"`
}

// CompletedYearStats holds aggregated stats for documents completed in a year (or all time when Year is 0).
type CompletedYearStats struct {
	Year          int
	DocumentCount int
	ReadingTime   string // estimated reading time (e.g. "2h 30m") from word count and user's words-per-minute
}
