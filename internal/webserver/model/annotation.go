package model

import "time"

// Annotation stores a user's selected text, location, and optional comment.
type Annotation struct {
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UserID    int       `gorm:"primaryKey;not null;autoIncrement:false" json:"-"`
	Slug      string    `gorm:"primaryKey;not null;index:idx_annotations_users_slug" json:"-"`
	// CFI identifies the annotated range's position in the document.
	CFI string `gorm:"column:cfi;primaryKey;not null" json:"cfi"`
	// Content stores the annotated text, so it can be shown without opening the book.
	Content string `gorm:"type:text;not null" json:"content"`
	Comment string `gorm:"type:text;not null;default:''" json:"comment"`
}

func (Annotation) TableName() string { return "annotations_users" }
