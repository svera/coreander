package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/svera/coreander/v4/internal/index"
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
	if err := repo.Update(42, "doc-slug", "epubcfi(/6/4[chap]!/4)", 37); err != nil {
		t.Fatalf("Update: %v", err)
	}
	var got Reading
	if err := db.Where("user_id = ? AND slug = ?", 42, "doc-slug").First(&got).Error; err != nil {
		t.Fatalf("First: %v", err)
	}
	if got.Percentage != 37 {
		t.Fatalf("Percentage = %d, want 37", got.Percentage)
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
	if err := repo.Update(1, "slug-a", "cfi-here", 0); err != nil {
		t.Fatal(err)
	}
	var got Reading
	if err := db.Where("user_id = ? AND slug = ?", 1, "slug-a").First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.Percentage != 0 {
		t.Fatalf("Percentage = %d, want 0", got.Percentage)
	}
}

func TestReadingRepositoryUpdateWithoutPercentageStoresZeroOnInsert(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Reading{}); err != nil {
		t.Fatal(err)
	}
	repo := &ReadingRepository{DB: db}
	if err := repo.Update(1, "slug-b", "only-pos", 0); err != nil {
		t.Fatal(err)
	}
	var got Reading
	if err := db.Where("user_id = ? AND slug = ?", 1, "slug-b").First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.Percentage != 0 {
		t.Fatalf("Percentage = %d, want 0 when omitted", got.Percentage)
	}
}

func TestReadingRepositoryUpdatePositionKeepsPercentageWhenResent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Reading{}); err != nil {
		t.Fatal(err)
	}
	repo := &ReadingRepository{DB: db}
	if err := repo.Update(2, "slug-c", "cfi-1", 50); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(2, "slug-c", "cfi-2", 50); err != nil {
		t.Fatal(err)
	}
	var got Reading
	if err := db.Where("user_id = ? AND slug = ?", 2, "slug-c").First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.Percentage != 50 {
		t.Fatalf("Percentage = %d, want 50 preserved", got.Percentage)
	}
}

type latestInProgressIdxStub struct {
	docs map[string]index.Document
}

func (s *latestInProgressIdxStub) Documents(slugs []string) (map[string]index.Document, error) {
	out := make(map[string]index.Document)
	for _, slug := range slugs {
		if d, ok := s.docs[slug]; ok {
			out[slug] = d
		}
	}
	return out, nil
}

func (s *latestInProgressIdxStub) TotalWordCount(slugs []string) (float64, error) {
	return 0, nil
}

func TestLatestInProgressReturnsAugmentedWithPercentage(t *testing.T) {
	const uid = 701
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Reading{}); err != nil {
		t.Fatal(err)
	}
	repo := &ReadingRepository{DB: db, Idx: &latestInProgressIdxStub{docs: map[string]index.Document{
		"a": {Slug: "a"},
		"b": {Slug: "b"},
		"c": {Slug: "c"},
	}}}
	if err := repo.Update(uid, "a", "pos-a", 42); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(uid, "b", "pos-b", 0); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(uid, "c", "pos-c", 150); err != nil {
		t.Fatal(err)
	}

	page, err := repo.Latest(uid, 1, 10)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if page.TotalHits() != 3 {
		t.Fatalf("total hits %d, want 3", page.TotalHits())
	}
	hits := page.Hits()
	if len(hits) != 3 {
		t.Fatalf("len(hits) %d, want 3", len(hits))
	}
	if hits[0].Slug != "c" || hits[0].ReadingPercentage != 100 {
		t.Errorf("first hit %+v", hits[0])
	}
	if hits[1].Slug != "b" || hits[1].ReadingPercentage != 0 {
		t.Errorf("second hit %+v", hits[1])
	}
	if hits[2].Slug != "a" || hits[2].ReadingPercentage != 42 {
		t.Errorf("third hit %+v", hits[2])
	}
}

func TestLatestInProgressSkipsMissingIndexDocuments(t *testing.T) {
	const uid = 702
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Reading{}); err != nil {
		t.Fatal(err)
	}
	repo := &ReadingRepository{DB: db, Idx: &latestInProgressIdxStub{docs: map[string]index.Document{
		"in-index": {Slug: "in-index"},
	}}}
	if err := repo.Update(uid, "in-index", "p", 10); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(uid, "ghost", "p2", 10); err != nil {
		t.Fatal(err)
	}
	page, err := repo.Latest(uid, 1, 10)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if page.TotalHits() != 2 {
		t.Fatalf("total %d, want 2", page.TotalHits())
	}
	hits := page.Hits()
	if len(hits) != 1 || hits[0].Slug != "in-index" {
		t.Fatalf("hits %+v", hits)
	}
}

func TestLatestRequiresIdx(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Reading{}); err != nil {
		t.Fatal(err)
	}
	repo := &ReadingRepository{DB: db, Idx: nil}
	_, err = repo.Latest(1, 1, 10)
	if err == nil {
		t.Fatal("expected error when Idx is nil")
	}
}
