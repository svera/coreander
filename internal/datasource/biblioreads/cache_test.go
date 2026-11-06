package biblioreads_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
)

func TestIncompleteBookResponsesAreNotCached(t *testing.T) {
	var hits atomic.Int32
	server := newCountingServer(t, http.StatusOK, `{"status":"Received","statusCode":200,"rating":"3.92"}`, &hits)
	client := newTestClient(biblioreads.WithInstances([]string{server.URL}))
	for range 2 {
		book, err := client.Book(context.Background(), "5898652")
		if err != nil || book.Rating != "3.92" {
			t.Fatalf("expected available score even without review identifiers, got %+v, %v", book, err)
		}
	}
	if hits.Load() != 2 {
		t.Fatal("incomplete book details must not be cached")
	}
}

func TestReviewsEmptyResponsesAreNotCached(t *testing.T) {
	var hits atomic.Int32
	server := newCountingServer(t, http.StatusOK, `{"status":"Received","statusCode":200,"totalCount":0,"reviews":[]}`, &hits)
	client := newTestClient(biblioreads.WithInstances([]string{server.URL}))
	for range 2 {
		response, err := client.Reviews(context.Background(), "work", 1, "", "")
		if err != nil || len(response.Reviews) != 0 {
			t.Fatalf("expected valid empty result, got %+v, %v", response, err)
		}
	}
	if hits.Load() != 2 {
		t.Fatal("empty results must not hide reviews behind the response cache")
	}
}

func TestReviewsLanguageCache(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		var payload struct {
			LanguageCode string `json:"languageCode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if payload.LanguageCode != "es" && payload.LanguageCode != "en" {
			t.Errorf("unexpected language %q", payload.LanguageCode)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(biblioreads.ReviewsResponse{
			Version: "BiblioReads v4.4.0", Status: "Received", StatusCode: http.StatusOK, TotalCount: 1,
			Reviews: []biblioreads.Review{{Text: payload.LanguageCode}},
		})
	}))
	defer server.Close()
	client := newTestClient(biblioreads.WithInstances([]string{server.URL}))
	for _, language := range []string{"es", "en", "es", "en"} {
		response, err := client.Reviews(context.Background(), "work", 1, "", language)
		if err != nil {
			t.Fatal(err)
		}
		if len(response.Reviews) != 1 || response.Reviews[0].Text != language {
			t.Fatalf("expected %s reviews, got %+v", language, response.Reviews)
		}
	}
	if hits.Load() != 2 {
		t.Fatalf("expected one cached response per language, got %d requests", hits.Load())
	}
}

func TestFailoverResponsesAreCached(t *testing.T) {
	t.Run("book", func(t *testing.T) {
		var hits atomic.Int32
		handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if hits.Add(1) == 1 {
				w.WriteHeader(http.StatusGatewayTimeout)
				_, _ = w.Write([]byte(`{"status":"Error - Book not found"}`))
				return
			}
			_, _ = w.Write([]byte(validBookResponse))
		})
		client := newFailoverClient(t, handler)

		for range 2 {
			book, err := client.Book(context.Background(), "5898652")
			if err != nil || book.Rating != "3.92" {
				t.Fatalf("expected recovered book, got %+v, %v", book, err)
			}
		}
		if hits.Load() != 2 {
			t.Fatalf("expected recovered book to be cached after failover, got %d requests", hits.Load())
		}
	})

	t.Run("reviews", func(t *testing.T) {
		var hits atomic.Int32
		fixture := readFixture(t, "reviews-scraper.json")
		handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if hits.Add(1) == 1 {
				_, _ = w.Write([]byte(`{}`))
				return
			}
			_, _ = w.Write(fixture)
		})
		client := newFailoverClient(t, handler)

		for range 2 {
			response, err := client.Reviews(context.Background(), "work", 1, "", "")
			if err != nil || len(response.Reviews) == 0 {
				t.Fatalf("expected recovered reviews, got %+v, %v", response, err)
			}
		}
		if hits.Load() != 2 {
			t.Fatalf("expected recovered reviews to be cached after failover, got %d requests", hits.Load())
		}
	})
}

func TestCache(t *testing.T) {
	t.Run("successful responses are cached per request", func(t *testing.T) {
		var hits atomic.Int32
		server := newCountingServer(t, http.StatusOK, `{"status":"Received","statusCode":200,"rating":"3.92","resourceID":"work-id","legacyBookID":5898652,"title":"La Reina Dragon","genres":["Fantasy"],"totalCount":1,"reviews":[{"id":"review-1"}]}`, &hits)
		client := newTestClient(biblioreads.WithInstances([]string{server.URL}))

		first, err := client.Book(context.Background(), "1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		first.Genres[0] = "modified"

		second, err := client.Book(context.Background(), "1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if hits.Load() != 1 {
			t.Errorf("expected cached response to be used, got %d requests", hits.Load())
		}
		if second.Title != "La Reina Dragon" || second.Genres[0] != "Fantasy" {
			t.Errorf("expected an unmodified copy of the cached response, got %+v", second)
		}

		if _, err := client.Book(context.Background(), "2"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := client.SearchBooks(context.Background(), "1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := client.Reviews(context.Background(), "kca://work/abc", 1, "", ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := client.Reviews(context.Background(), "kca://work/abc", 1, "token", ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for range 2 {
			if _, err := client.Author(context.Background(), "1"); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		}
		if hits.Load() != 6 {
			t.Errorf("expected different requests not to share cache entries, got %d requests", hits.Load())
		}
	})

	t.Run("entries expire after the TTL", func(t *testing.T) {
		var hits atomic.Int32
		server := newCountingServer(t, http.StatusOK, validBookResponse, &hits)
		client := newTestClient(biblioreads.WithInstances([]string{server.URL}), biblioreads.WithCacheTTL(100*time.Millisecond))

		for range 3 {
			if _, err := client.Book(context.Background(), "1"); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		}
		if hits.Load() != 1 {
			t.Fatalf("expected 1 request before expiry, got %d", hits.Load())
		}

		time.Sleep(150 * time.Millisecond)
		if _, err := client.Book(context.Background(), "1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if hits.Load() != 2 {
			t.Errorf("expected expired entry to be refreshed, got %d requests", hits.Load())
		}
	})

	t.Run("caching can be disabled", func(t *testing.T) {
		var hits atomic.Int32
		server := newCountingServer(t, http.StatusOK, validBookResponse, &hits)
		client := newTestClient(biblioreads.WithInstances([]string{server.URL}), biblioreads.WithCacheTTL(0))

		for range 3 {
			if _, err := client.Book(context.Background(), "1"); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		}
		if hits.Load() != 3 {
			t.Errorf("expected 3 requests with caching disabled, got %d", hits.Load())
		}
	})
}
