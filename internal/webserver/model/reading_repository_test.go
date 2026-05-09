package model

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestReadingRepositoryUpdatePersistsPercentage(t *testing.T) {
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
	if got.Percentage == nil || *got.Percentage != 37 {
		t.Fatalf("Percentage = %v, want 37", got.Percentage)
	}
}

func TestReadingRepositoryUpdatePersistsZeroPercentage(t *testing.T) {
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
	if got.Percentage == nil || *got.Percentage != 0 {
		t.Fatalf("Percentage = %v, want 0", got.Percentage)
	}
}

func TestReadingRepositoryUpdateWithoutPercentageLeavesColumnUnsetOnInsert(t *testing.T) {
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
	if got.Percentage != nil {
		t.Fatalf("Percentage = %v, want nil on first insert", got.Percentage)
	}
}

func TestReadingRepositoryUpdatePositionWithoutPercentageKeepsExisting(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Reading{}); err != nil {
		t.Fatal(err)
	}
	repo := &ReadingRepository{DB: db}
	fifty := 50
	if err := repo.Update(2, "slug-c", "cfi-1", &fifty); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(2, "slug-c", "cfi-2", nil); err != nil {
		t.Fatal(err)
	}
	var got Reading
	if err := db.Where("user_id = ? AND slug = ?", 2, "slug-c").First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.Percentage == nil || *got.Percentage != 50 {
		t.Fatalf("Percentage = %v, want 50 preserved", got.Percentage)
	}
}

func TestReadingPercentageBySlugs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Reading{}); err != nil {
		t.Fatal(err)
	}
	repo := &ReadingRepository{DB: db}
	p42, p150 := 42, 150
	if err := repo.Update(1, "a", "pos-a", &p42); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(1, "b", "pos-b", nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(1, "c", "pos-c", &p150); err != nil {
		t.Fatal(err)
	}
	p10 := 10
	completedAt := time.Now()
	if err := db.Create(&Reading{
		UserID:      1,
		Slug:        "d",
		Position:    "x",
		Percentage:  &p10,
		CompletedOn: &completedAt,
	}).Error; err != nil {
		t.Fatal(err)
	}

	got, err := repo.ReadingPercentageBySlugs(1, []string{"a", "b", "c", "d", "missing"})
	if err != nil {
		t.Fatalf("ReadingPercentageBySlugs: %v", err)
	}
	if got["a"] != 42 {
		t.Errorf("a = %d, want 42", got["a"])
	}
	if got["b"] != 0 {
		t.Errorf("b = %d, want 0 (nil percentage)", got["b"])
	}
	if got["c"] != 100 {
		t.Errorf("c = %d, want 100 (clamped)", got["c"])
	}
	if got["missing"] != 0 {
		t.Errorf("missing = %d, want 0", got["missing"])
	}
}
