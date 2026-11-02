package series

import (
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestParseGoodReadsScoreSort(t *testing.T) {
	for _, direction := range []string{"asc", "desc"} {
		t.Run(direction, func(t *testing.T) {
			app := fiber.New()
			app.Get("/", func(c fiber.Ctx) error {
				key := "GoodReadsRatingValue"
				if direction == "desc" {
					key = "-" + key
				}
				want := []string{key, "SeriesIndex"}
				if got := (&Controller{}).parseSortBy(c); !slices.Equal(got, want) {
					t.Errorf("got %v, want %v", got, want)
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
