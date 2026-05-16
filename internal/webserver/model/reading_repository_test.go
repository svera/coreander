package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/svera/coreander/v4/internal/index"
	"gorm.io/gorm"
)

func newTestReadingRepo(t *testing.T) *ReadingRepository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Reading{}); err != nil {
		t.Fatal(err)
	}
	return &ReadingRepository{DB: db}
}

func mustUpdateReading(t *testing.T, repo *ReadingRepository, userID int, slug, position string, pct int) {
	t.Helper()
	if err := repo.Update(userID, slug, position, pct); err != nil {
		t.Fatalf("Update: %v", err)
	}
}

func firstReading(t *testing.T, db *gorm.DB, userID int, slug string) Reading {
	t.Helper()
	var got Reading
	if err := db.Where("user_id = ? AND slug = ?", userID, slug).First(&got).Error; err != nil {
		t.Fatalf("First: %v", err)
	}
	return got
}

func TestReadingRepositoryUpdatePersistsZeroPercentage(t *testing.T) {
	repo := newTestReadingRepo(t)
	mustUpdateReading(t, repo, 1, "slug-a", "cfi-here", 0)
	got := firstReading(t, repo.DB, 1, "slug-a")
	if got.Percentage != 0 {
		t.Fatalf("Percentage = %d, want 0", got.Percentage)
	}
}

func TestReadingRepositoryUpdateWithoutPercentageStoresZeroOnInsert(t *testing.T) {
	repo := newTestReadingRepo(t)
	mustUpdateReading(t, repo, 1, "slug-b", "only-pos", 0)
	got := firstReading(t, repo.DB, 1, "slug-b")
	if got.Percentage != 0 {
		t.Fatalf("Percentage = %d, want 0 when omitted", got.Percentage)
	}
}

func TestReadingRepositoryUpdatePositionKeepsPercentageWhenResent(t *testing.T) {
	repo := newTestReadingRepo(t)
	mustUpdateReading(t, repo, 2, "slug-c", "cfi-1", 50)
	mustUpdateReading(t, repo, 2, "slug-c", "cfi-2", 50)
	got := firstReading(t, repo.DB, 2, "slug-c")
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
	repo := newTestReadingRepo(t)
	repo.Idx = &latestInProgressIdxStub{docs: map[string]index.Document{
		"a": {Slug: "a"},
		"b": {Slug: "b"},
		"c": {Slug: "c"},
	}}
	mustUpdateReading(t, repo, uid, "a", "pos-a", 42)
	mustUpdateReading(t, repo, uid, "b", "pos-b", 0)
	mustUpdateReading(t, repo, uid, "c", "pos-c", 150)

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
	repo := newTestReadingRepo(t)
	repo.Idx = &latestInProgressIdxStub{docs: map[string]index.Document{
		"in-index": {Slug: "in-index"},
	}}
	mustUpdateReading(t, repo, uid, "in-index", "p", 10)
	mustUpdateReading(t, repo, uid, "ghost", "p2", 10)
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
	repo := newTestReadingRepo(t)
	_, err := repo.Latest(1, 1, 10)
	if err == nil {
		t.Fatal("expected error when Idx is nil")
	}
}
