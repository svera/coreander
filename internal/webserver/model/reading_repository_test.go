package model

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestReadingRepositoryUpdatePersistsFraction(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Reading{}); err != nil {
		t.Fatal(err)
	}
	repo := &ReadingRepository{DB: db}
	f := 0.37
	if err := repo.Update(1, "slug-a", "epubcfi(/6/4)", &f); err != nil {
		t.Fatal(err)
	}
	var got Reading
	if err := db.Where("user_id = ? AND slug = ?", 1, "slug-a").First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.Fraction == nil || *got.Fraction != 0.37 {
		t.Fatalf("Fraction = %v, want 0.37", got.Fraction)
	}
}

func TestReadingRepositoryUpdatePositionWithoutFractionKeepsExistingFraction(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Reading{}); err != nil {
		t.Fatal(err)
	}
	repo := &ReadingRepository{DB: db}
	f := 0.5
	if err := repo.Update(2, "slug-b", "cfi-1", &f); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(2, "slug-b", "cfi-2", nil); err != nil {
		t.Fatal(err)
	}
	var got Reading
	if err := db.Where("user_id = ? AND slug = ?", 2, "slug-b").First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.Fraction == nil || *got.Fraction != 0.5 {
		t.Fatalf("Fraction = %v, want 0.5 preserved", got.Fraction)
	}
	if got.Position != "cfi-2" {
		t.Fatalf("Position = %q", got.Position)
	}
}

func TestReadingRepositoryUpdateWithoutFractionLeavesColumnUnsetOnInsert(t *testing.T) {
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
	if got.Fraction != nil {
		t.Fatalf("Fraction = %v, want nil on first insert", got.Fraction)
	}
}

func TestReadingFractionBySlugs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Reading{}); err != nil {
		t.Fatal(err)
	}
	repo := &ReadingRepository{DB: db}
	a := 0.42
	if err := repo.Update(1, "a", "p", &a); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(1, "b", "p2", nil); err != nil {
		t.Fatal(err)
	}
	c := 2.0
	if err := repo.Update(1, "c", "p3", &c); err != nil {
		t.Fatal(err)
	}
	completedAt := time.Now()
	p10 := 0.1
	if err := db.Create(&Reading{
		UserID:      1,
		Slug:        "d",
		Position:    "p",
		Fraction:    &p10,
		CompletedOn: &completedAt,
	}).Error; err != nil {
		t.Fatal(err)
	}

	got, err := repo.ReadingFractionBySlugs(1, []string{"a", "b", "c", "d", "missing"})
	if err != nil {
		t.Fatalf("ReadingFractionBySlugs: %v", err)
	}
	if got["a"] != 0.42 {
		t.Errorf("a = %v", got["a"])
	}
	if _, ok := got["b"]; ok {
		t.Errorf("b should be omitted (nil fraction)")
	}
	if got["c"] != 1 {
		t.Errorf("c = %v, want 1 (clamped)", got["c"])
	}
	if _, ok := got["d"]; ok {
		t.Errorf("completed d should be omitted")
	}
	if _, ok := got["missing"]; ok {
		t.Errorf("missing should be omitted")
	}
}
