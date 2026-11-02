package search

import (
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v5/internal/index"
)

func TestParseReviewScoreSort(t *testing.T) {
	for _, direction := range []string{"asc", "desc"} {
		t.Run(direction, func(t *testing.T) {
			app := fiber.New()
			app.Get("/", func(c fiber.Ctx) error {
				fields, err := ParseDocumentSearchQuery(c, 250)
				if err != nil {
					t.Error(err)
					return c.SendStatus(fiber.StatusBadRequest)
				}
				key := index.ScoreValueSortField
				if direction == "desc" {
					key = "-" + key
				}
				if !slices.Equal(fields.SortBy, []string{key}) {
					t.Errorf("got %v, want [%s]", fields.SortBy, key)
				}
				return c.SendStatus(fiber.StatusOK)
			})
			response, err := app.Test(httptest.NewRequest("GET", "/?sort-by=review-score-"+direction, nil))
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != fiber.StatusOK {
				t.Fatalf("unexpected status %d", response.StatusCode)
			}
		})
	}
}
