package model

import "time"

// UserAnnotation links a user to a document (identified by its slug) for storing the user's text annotations.
type UserAnnotation struct {
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
	UserID    int       `gorm:"primaryKey;not null;autoIncrement:false" json:"-"`
	Slug      string    `gorm:"primaryKey;not null;index:idx_annotations_users_slug" json:"-"`
	// CFI identifies the annotated range's position in the document.
	CFI string `gorm:"column:cfi;primaryKey;not null" json:"cfi"`
	// Content stores the annotated text, so it can be shown without opening the book.
	Content string `gorm:"type:text;not null" json:"content"`
}

func (UserAnnotation) TableName() string { return "annotations_users" }
