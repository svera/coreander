package model

import (
	"errors"
	"time"
)

var ErrShareAlreadyExists = errors.New("share already exists")

type Highlight struct {
	CreatedAt  time.Time
	UpdatedAt  time.Time
	UserID     int    `gorm:"primaryKey;index;not null"`
	Path       string `gorm:"primaryKey;index;not null"`
	SharedByID *int   `gorm:"index"`
	Comment    string `gorm:"type:text"`
}
