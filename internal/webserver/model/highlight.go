package model

import "time"

type Highlight struct {
	CreatedAt       time.Time
	UpdatedAt       time.Time
	UserID          int    `gorm:"primaryKey;index;not null"`
	Path            string `gorm:"primaryKey;index;not null"`
	SharedByID      *int   `gorm:"index"`
	Comment         string `gorm:"type:text"`
	SharedBy        *User          `gorm:"foreignKey:SharedByID"`
}

