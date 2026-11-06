package biblioreads_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
)

func TestBookResponseRecovery(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status int
		body   string
	}{
		{"JSON server error", http.StatusGatewayTimeout, `{"status":"Error - Book not found"}`},
		{"invalid success", http.StatusOK, `{}`},
		{"application error", http.StatusOK, `{"status":"Error","statusCode":503}`},
		{"missing score", http.StatusOK, `{"status":"Received","statusCode":200,"resourceID":"work-id","legacyBookID":5898652,"title":"Incomplete result"}`},
		{"missing review identifiers", http.StatusOK, `{"status":"Received","statusCode":200,"rating":"3.92"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var hits atomic.Int32
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if hits.Add(1) == 1 {
					w.WriteHeader(tt.status)
					_, _ = w.Write([]byte(tt.body))
					return
				}
				_, _ = w.Write([]byte(validBookResponse))
			})
			client := newFailoverClient(t, handler)
			book, err := client.Book(context.Background(), "5898652-la-cinta-roja")
			if err != nil || book.Rating != "3.92" || book.ResourceID != "work-id" || book.LegacyBookID != 5898652 {
				t.Fatalf("expected complete book after failover, got %+v, %v", book, err)
			}
			if book.Title != "" {
				t.Fatalf("fields from previous attempts must not survive failover, got title %q", book.Title)
			}
			if hits.Load() != 2 {
				t.Fatalf("expected both instances to be tried, got %d requests", hits.Load())
			}
		})
	}
}

func TestIncompleteBookFallbackPrefersScore(t *testing.T) {
	scored := `{"status":"Received","statusCode":200,"rating":"3.92","title":"Scored result"}`
	unscored := `{"status":"Received","statusCode":200,"resourceID":"work-id","legacyBookID":5898652,"title":"Unscored result"}`
	for _, scoredFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("scored first=%t", scoredFirst), func(t *testing.T) {
			var hits atomic.Int32
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := unscored
				if (hits.Add(1)%2 == 1) == scoredFirst {
					body = scored
				}
				_, _ = w.Write([]byte(body))
			})
			client := newFailoverClient(t, handler)
			for range 2 {
				book, err := client.Book(context.Background(), "5898652")
				if err != nil || book.Rating != "3.92" || book.Title != "Scored result" {
					t.Fatalf("expected scored fallback, got %+v, %v", book, err)
				}
				if book.ResourceID != "" || book.LegacyBookID != 0 {
					t.Fatalf("fields from separate incomplete responses must not be combined, got %+v", book)
				}
			}
			if hits.Load() != 4 {
				t.Fatalf("expected both instances tried without caching incomplete results, got %d requests", hits.Load())
			}
		})
	}
}

func TestReviewsResponseRecovery(t *testing.T) {
	fixture := readFixture(t, "reviews-scraper.json")
	for _, tt := range []struct {
		name string
		body string
	}{
		{name: "invalid response", body: `{}`},
		{name: "empty response", body: `{"status":"Received","statusCode":200,"totalCount":0,"reviews":[]}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var hits atomic.Int32
			client := newFailoverClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if hits.Add(1) == 1 {
					_, _ = w.Write([]byte(tt.body))
					return
				}
				_, _ = w.Write(fixture)
			}))
			reviews, err := client.Reviews(context.Background(), "work", 1, "", "")
			if err != nil || len(reviews.Reviews) == 0 {
				t.Fatalf("expected reviews after failover, got %+v, %v", reviews, err)
			}
			if hits.Load() != 2 {
				t.Fatalf("expected both instances to be tried, got %d requests", hits.Load())
			}
		})
	}
}

func TestReviewsEmptyFallbackSurvivesInvalidResponse(t *testing.T) {
	var hits atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := `{"version":"BiblioReads v4.4.0","status":"Received","statusCode":200,"totalCount":0,"reviews":[],"nextPageToken":"empty-page"}`
		if hits.Add(1) > 1 {
			body = `{"reviews":[{"text":"Unfiltered review"}],"nextPageToken":"invalid-page"}`
		}
		_, _ = w.Write([]byte(body))
	})
	client := newFailoverClient(t, handler)
	response, err := client.Reviews(context.Background(), "work", 1, "", "es")
	if err != nil || len(response.Reviews) != 0 || response.NextPageToken != "empty-page" {
		t.Fatalf("expected original empty fallback without fields from the invalid response, got %+v, %v", response, err)
	}
	if hits.Load() != 2 {
		t.Fatalf("expected both instances tried, got %d requests", hits.Load())
	}
}

func TestReviewsLanguageFailsOverFromLegacyInstance(t *testing.T) {
	var hits atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := biblioreads.ReviewsResponse{
			Status: "Received", StatusCode: http.StatusOK, TotalCount: 1,
			Reviews: []biblioreads.Review{{Text: "An unfiltered English review"}},
		}
		if hits.Add(1) > 1 {
			response.Version = "BiblioReads v4.4.0"
			response.Reviews[0].Text = "Una reseña en español"
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("encode response: %v", err)
		}
	})
	client := newFailoverClient(t, handler)
	response, err := client.Reviews(context.Background(), "work", 1, "", "es")
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Reviews) != 1 || response.Reviews[0].Text != "Una reseña en español" {
		t.Fatalf("expected Spanish reviews rather than legacy unfiltered results, got %+v", response)
	}
	if hits.Load() != 2 {
		t.Fatalf("expected both instances to be tried, got %d requests", hits.Load())
	}
}

func TestFailover(t *testing.T) {
	t.Run("unavailable instances are skipped", func(t *testing.T) {
		var unavailableHits, rateLimitedHits, htmlHits, workingHits atomic.Int32
		unavailable := newCountingServer(t, http.StatusBadGateway, "Bad Gateway", &unavailableHits)
		rateLimited := newCountingServer(t, http.StatusTooManyRequests, `{"status":"Too many requests"}`, &rateLimitedHits)
		html := newCountingServer(t, http.StatusOK, "<html>Parked domain</html>", &htmlHits)
		working := newCountingServer(t, http.StatusOK, `{"status":"Received","statusCode":200,"rating":"3.92","title":"La Reina Dragon"}`, &workingHits)
		down := httptest.NewServer(http.NotFoundHandler())
		down.Close()

		instances := []string{down.URL, unavailable.URL, rateLimited.URL, html.URL, working.URL}
		for range 20 {
			client := newTestClient(biblioreads.WithInstances(instances), biblioreads.WithCacheTTL(0))
			res, err := client.Book(context.Background(), "12207444")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Title != "La Reina Dragon" {
				t.Fatalf("unexpected title %q", res.Title)
			}
		}
		if workingHits.Load() != 20 {
			t.Errorf("expected working instance to be hit 20 times, got %d", workingHits.Load())
		}
	})

	t.Run("requests start at different instances", func(t *testing.T) {
		var firstHits, secondHits atomic.Int32
		first := newCountingServer(t, http.StatusOK, validBookResponse, &firstHits)
		second := newCountingServer(t, http.StatusOK, validBookResponse, &secondHits)

		client := newTestClient(biblioreads.WithInstances([]string{first.URL, second.URL}), biblioreads.WithCacheTTL(0))
		for range 100 {
			if _, err := client.Book(context.Background(), "1"); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		}
		if firstHits.Load() == 0 || secondHits.Load() == 0 {
			t.Errorf("expected load to be spread, got %d and %d hits", firstHits.Load(), secondHits.Load())
		}
	})

	t.Run("JSON error responses are not retried", func(t *testing.T) {
		var hits atomic.Int32
		first := newCountingServer(t, http.StatusNotFound, `{"status":"Error - Invalid Query"}`, &hits)
		second := newCountingServer(t, http.StatusNotFound, `{"status":"Error - Invalid Query"}`, &hits)

		client := newTestClient(biblioreads.WithInstances([]string{first.URL, second.URL}), biblioreads.WithCacheTTL(0))
		_, err := client.Book(context.Background(), "0")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if errors.Is(err, biblioreads.ErrUnavailable) {
			t.Errorf("expected a non-ErrUnavailable error, got %v", err)
		}
		if hits.Load() != 1 {
			t.Errorf("expected a single request, got %d", hits.Load())
		}
	})

	t.Run("error when all instances fail", func(t *testing.T) {
		var hits atomic.Int32
		first := newCountingServer(t, http.StatusServiceUnavailable, "", &hits)
		second := newCountingServer(t, http.StatusServiceUnavailable, "", &hits)

		client := newTestClient(biblioreads.WithInstances([]string{first.URL, second.URL}), biblioreads.WithCacheTTL(0))
		if _, err := client.Book(context.Background(), "1"); !errors.Is(err, biblioreads.ErrUnavailable) {
			t.Fatalf("expected ErrUnavailable, got %v", err)
		}
		if hits.Load() != 2 {
			t.Errorf("expected both instances to be tried, got %d requests", hits.Load())
		}
	})

	t.Run("cancelled context stops failover", func(t *testing.T) {
		var hits atomic.Int32
		first := newCountingServer(t, http.StatusOK, `{}`, &hits)
		second := newCountingServer(t, http.StatusOK, `{}`, &hits)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		client := newTestClient(biblioreads.WithInstances([]string{first.URL, second.URL}), biblioreads.WithCacheTTL(0))
		if _, err := client.Book(ctx, "1"); err == nil {
			t.Fatal("expected error, got nil")
		}
		if hits.Load() != 0 {
			t.Errorf("expected no requests, got %d", hits.Load())
		}
	})
}

func TestCooldown(t *testing.T) {
	t.Run("errors put the instance into cooldown", func(t *testing.T) {
		var hits atomic.Int32
		server := newCountingServer(t, http.StatusGatewayTimeout, `{"status":"Error - Book not found"}`, &hits)
		client := newTestClient(biblioreads.WithInstances([]string{server.URL}))

		for range 3 {
			if _, err := client.Book(context.Background(), "0"); err == nil {
				t.Fatal("expected error, got nil")
			}
		}
		if hits.Load() != 1 {
			t.Errorf("expected subsequent requests to respect cooldown, got %d requests", hits.Load())
		}
	})

	t.Run("failing instance is skipped on later requests", func(t *testing.T) {
		var badHits, goodHits atomic.Int32
		bad := newCountingServer(t, http.StatusBadGateway, "<html>Bad Gateway</html>", &badHits)
		good := newCountingServer(t, http.StatusOK, validBookResponse, &goodHits)

		client := newTestClient(biblioreads.WithInstances([]string{bad.URL, good.URL}), biblioreads.WithCacheTTL(0))
		for range 20 {
			if _, err := client.Book(context.Background(), "1"); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		}
		if badHits.Load() > 1 {
			t.Errorf("expected failing instance to be hit at most once, got %d", badHits.Load())
		}
		if goodHits.Load() != 20 {
			t.Errorf("expected working instance to be hit 20 times, got %d", goodHits.Load())
		}
	})

	t.Run("instance is used again after Retry-After", func(t *testing.T) {
		var limitedHits, otherHits atomic.Int32
		limited := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limitedHits.Add(1) == 1 {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			_, _ = w.Write([]byte(validBookResponse))
		}))
		t.Cleanup(limited.Close)
		other := newCountingServer(t, http.StatusOK, validBookResponse, &otherHits)

		client := newTestClient(biblioreads.WithInstances([]string{limited.URL, other.URL}), biblioreads.WithCacheTTL(0))
		for limitedHits.Load() == 0 {
			if _, err := client.Book(context.Background(), "1"); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		}
		for range 20 {
			if _, err := client.Book(context.Background(), "1"); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		}
		if limitedHits.Load() != 1 {
			t.Fatalf("expected rate limited instance to be skipped during cooldown, got %d hits", limitedHits.Load())
		}

		time.Sleep(1100 * time.Millisecond)
		for range 20 {
			if _, err := client.Book(context.Background(), "1"); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		}
		if limitedHits.Load() == 1 {
			t.Error("expected rate limited instance to be used again after its cooldown")
		}
	})

	t.Run("respects cooldown when all instances are cooling down", func(t *testing.T) {
		var hits atomic.Int32
		flaky := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if hits.Add(1) == 1 {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			_, _ = w.Write([]byte(validBookResponse))
		}))
		t.Cleanup(flaky.Close)

		client := newTestClient(biblioreads.WithInstances([]string{flaky.URL}), biblioreads.WithCacheTTL(0))
		if _, err := client.Book(context.Background(), "1"); err == nil {
			t.Fatal("expected error on first request, got nil")
		}
		if _, err := client.Book(context.Background(), "1"); !errors.Is(err, biblioreads.ErrUnavailable) {
			t.Fatalf("expected unavailable during cooldown, got %v", err)
		}
		time.Sleep(1100 * time.Millisecond)
		if _, err := client.Book(context.Background(), "1"); err != nil {
			t.Fatalf("expected cooling down instance to be probed, got %v", err)
		}
	})

	t.Run("JSON client error responses don't trigger cooldown", func(t *testing.T) {
		var firstHits, secondHits atomic.Int32
		notFound := `{"status":"Error - Book not found"}`
		first := newCountingServer(t, http.StatusNotFound, notFound, &firstHits)
		second := newCountingServer(t, http.StatusNotFound, notFound, &secondHits)

		client := newTestClient(biblioreads.WithInstances([]string{first.URL, second.URL}), biblioreads.WithCacheTTL(0))
		for range 20 {
			if _, err := client.Book(context.Background(), "0"); err == nil {
				t.Fatal("expected error, got nil")
			}
		}
		if firstHits.Load()+secondHits.Load() != 20 {
			t.Errorf("expected one request per call, got %d", firstHits.Load()+secondHits.Load())
		}
		if firstHits.Load() == 0 || secondHits.Load() == 0 {
			t.Errorf("expected no instance to be benched, got %d and %d hits", firstHits.Load(), secondHits.Load())
		}
	})

	t.Run("cancelled requests don't trigger cooldown", func(t *testing.T) {
		var firstHits, secondHits atomic.Int32
		first := newCountingServer(t, http.StatusOK, validBookResponse, &firstHits)
		second := newCountingServer(t, http.StatusOK, validBookResponse, &secondHits)

		client := newTestClient(biblioreads.WithInstances([]string{first.URL, second.URL}), biblioreads.WithCacheTTL(0))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		for range 5 {
			_, _ = client.Book(ctx, "1")
		}
		for range 40 {
			if _, err := client.Book(context.Background(), "1"); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		}
		if firstHits.Load() == 0 || secondHits.Load() == 0 {
			t.Errorf("expected no instance to be benched, got %d and %d hits", firstHits.Load(), secondHits.Load())
		}
	})
}
