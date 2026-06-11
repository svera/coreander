package result_test

import (
	"testing"

	"github.com/svera/coreander/v4/internal/result"
)

func TestPaginateFullList(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}

	page := result.Paginate(2, 2, len(items), items)
	if got := page.Hits(); len(got) != 2 || got[0] != 3 || got[1] != 4 {
		t.Fatalf("expected page [3 4], got %#v", got)
	}
	if page.TotalHits() != 5 || page.Page() != 2 {
		t.Fatalf("unexpected metadata: total=%d page=%d", page.TotalHits(), page.Page())
	}
}

func TestPaginatePrePaginatedWindow(t *testing.T) {
	pageWindow := []string{"c", "d"}

	page := result.Paginate(2, 2, 5, pageWindow)
	if got := page.Hits(); len(got) != 2 || got[0] != "c" || got[1] != "d" {
		t.Fatalf("expected pre-paginated window unchanged, got %#v", got)
	}
}

func TestPaginateEmptyPage(t *testing.T) {
	items := []int{1, 2, 3}

	page := result.Paginate(2, 3, len(items), items)
	if len(page.Hits()) != 0 {
		t.Fatalf("expected empty page, got %#v", page.Hits())
	}
}
