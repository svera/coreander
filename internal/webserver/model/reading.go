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

// CompletedYearStats holds aggregated stats for documents completed in a year (or all time when Year is 0).
type CompletedYearStats struct {
	Year          int
	DocumentCount int
	ReadingTime   string // estimated reading time (e.g. "2h 30m") from word count and user's words-per-minute
}

// CompletedYearStatsRow is the repository result for CompletedStatsByYear (includes Slugs for word count lookup).
type CompletedYearStatsRow struct {
	Year          int
	DocumentCount int
	Slugs         []string
}
