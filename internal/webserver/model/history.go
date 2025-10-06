package model

import (
	"time"
)

const (
	HistoryActionRead      = 1
	HistoryActionSend      = 2
	HistoryActionHighlight = 3
	HistoryActionDownload  = 4
)

type History struct {
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
	UserID    int       `gorm:"primaryKey"`
	Path      string    `gorm:"primaryKey"`
	Action    int       `gorm:"primaryKey"`
}

func (History) TableName() string {
	return "history"
}
