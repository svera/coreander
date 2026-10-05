package infrastructure_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/svera/coreander/v5/internal/webserver/infrastructure"
	"github.com/svera/coreander/v5/internal/webserver/model"
	"gorm.io/gorm"
)

type legacyAnnotation struct {
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
	UserID    int       `gorm:"primaryKey;not null;autoIncrement:false"`
	Slug      string    `gorm:"primaryKey;not null;index:idx_annotations_users_slug"`
	CFI       string    `gorm:"column:cfi;primaryKey;not null"`
	Content   string    `gorm:"type:text;not null"`
}

func (legacyAnnotation) TableName() string { return "annotations_users" }

func TestConnect_AddsAnnotationComment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "annotations.db")
	seedLegacySchema(t, path)
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&legacyAnnotation{}); err != nil {
		t.Fatal(err)
	}
	original := legacyAnnotation{
		UserID: 1, Slug: "some-book", CFI: "epubcfi(/6/2)", Content: "Existing passage",
	}
	if err := db.Create(&original).Error; err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}

	db = infrastructure.Connect(path, 250)
	sqlDB, err = db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	var migrated model.Annotation
	if err := db.First(&migrated).Error; err != nil {
		t.Fatal(err)
	}
	if migrated.UserID != original.UserID || migrated.Slug != original.Slug ||
		migrated.CFI != original.CFI || migrated.Content != original.Content ||
		!migrated.CreatedAt.Equal(original.CreatedAt) ||
		migrated.Comment != "" {
		t.Fatalf("migration did not preserve the annotation: %+v", migrated)
	}
	repository := model.AnnotationRepository{DB: db}
	if err := repository.Save(migrated.UserID, migrated.Slug, migrated.CFI, migrated.Content, "New comment"); err != nil {
		t.Fatal(err)
	}
	annotations, err := repository.List(migrated.UserID, migrated.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if len(annotations) != 1 || annotations[0].Comment != "New comment" {
		t.Fatalf("comment was not persisted after migration: %+v", annotations)
	}
}
