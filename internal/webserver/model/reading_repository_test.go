package model

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestReadingRepositoryUpdatePersistsProgressPercent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Reading{}); err != nil {
		t.Fatal(err)
	}
	repo := &ReadingRepository{DB: db}

	p := 37
	if err := repo.Update(42, "doc-slug", "epubcfi(/6/4[chap]!/4)", &p); err != nil {
		t.Fatalf("Update: %v", err)
	}

	var got Reading
	if err := db.Where("user_id = ? AND slug = ?", 42, "doc-slug").First(&got).Error; err != nil {
		t.Fatalf("First: %v", err)
	}
	if got.Progress == nil || *got.Progress != 37 {
		t.Fatalf("Progress = %v, want 37", got.Progress)
	}
	if got.Position != "epubcfi(/6/4[chap]!/4)" {
		t.Fatalf("Position = %q", got.Position)
	}
}

func TestReadingRepositoryUpdatePersistsZeroProgress(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Reading{}); err != nil {
		t.Fatal(err)
	}
	repo := &ReadingRepository{DB: db}
	z := 0
	if err := repo.Update(1, "slug-a", "cfi-here", &z); err != nil {
		t.Fatal(err)
	}
	var got Reading
	if err := db.Where("user_id = ? AND slug = ?", 1, "slug-a").First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.Progress == nil || *got.Progress != 0 {
		t.Fatalf("Progress = %v, want 0", got.Progress)
	}
}

func TestReadingRepositoryUpdateWithoutProgressLeavesColumnUnsetOnInsert(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Reading{}); err != nil {
		t.Fatal(err)
	}
	repo := &ReadingRepository{DB: db}
	if err := repo.Update(1, "slug-b", "only-pos", nil); err != nil {
		t.Fatal(err)
	}
	var got Reading
	if err := db.Where("user_id = ? AND slug = ?", 1, "slug-b").First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.Position != "only-pos" {
		t.Fatalf("Position = %q", got.Position)
	}
	if got.Progress != nil {
		t.Fatalf("Progress = %v, want nil on first insert", got.Progress)
	}
}

func TestReadingProgressPercentBySlugs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Reading{}); err != nil {
		t.Fatal(err)
	}
	repo := &ReadingRepository{DB: db}
	p42 := 42
	p150 := 150
	if err := repo.Update(1, "a", "pos-a", &p42); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(1, "b", "pos-b", nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(1, "c", "pos-c", &p150); err != nil {
		t.Fatal(err)
	}
	// Completed row must not appear in map for in-progress query
	p10 := 10
	completedAt := time.Now()
	if err := db.Create(&Reading{
		UserID:      1,
		Slug:        "d",
		Position:    "x",
		Progress:    &p10,
		CompletedOn: &completedAt,
	}).Error; err != nil {
		t.Fatal(err)
	}

	got, err := repo.ReadingProgressPercentBySlugs(1, []string{"a", "b", "c", "d", "missing"})
	if err != nil {
		t.Fatalf("ReadingProgressPercentBySlugs: %v", err)
	}
	if got["a"] != 42 {
		t.Errorf("a = %d, want 42", got["a"])
	}
	if got["b"] != 0 {
		t.Errorf("b = %d, want 0 (nil progress)", got["b"])
	}
	if got["c"] != 100 {
		t.Errorf("c = %d, want 100 (clamped)", got["c"])
	}
	if _, ok := got["d"]; ok {
		t.Errorf("completed slug d should be absent from map")
	}
	if got["missing"] != 0 {
		t.Errorf("missing = %d, want 0", got["missing"])
	}
}
