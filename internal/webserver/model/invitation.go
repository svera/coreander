package model

import "time"

// Invitation represents a user invitation
type Invitation struct {
	ID         uint   `gorm:"primarykey"`
	Email      string `gorm:"type:text collate nocase; not null; uniqueIndex"`
	UUID       string `gorm:"uniqueIndex; not null"`
	ValidUntil time.Time
}
