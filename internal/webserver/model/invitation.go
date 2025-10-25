package model

import "time"

// Invitation represents a user invitation
type Invitation struct {
	ID         uint `gorm:"primarykey"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Email      string `gorm:"uniqueIndex; not null"`
	UUID       string `gorm:"uniqueIndex; not null"`
	ValidUntil time.Time
}
