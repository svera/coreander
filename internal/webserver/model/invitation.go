package model

import "time"

// Invitation represents a user invitation
type Invitation struct {
	ID         uint   `gorm:"primarykey"`
	Email      string `gorm:"uniqueIndex; not null"`
	UUID       string `gorm:"uniqueIndex; not null"`
	ValidUntil time.Time
}
