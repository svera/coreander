package biblioreads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const validBookResponse = `{"status":"Received","statusCode":200,"rating":"3.92","resourceID":"work-id","legacyBookID":5898652}`

func newTestClient(opts ...Option) *Client {
	return NewClient(append([]Option{WithRequestInterval(0)}, opts...)...)
}

func newFailoverClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	first := httptest.NewServer(handler)
	t.Cleanup(first.Close)
	second := httptest.NewServer(handler)
	t.Cleanup(second.Close)
	return newTestClient(WithInstances([]string{first.URL, second.URL}))
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	contents, err := os.ReadFile("fixtures/" + name)
	if err != nil {
		t.Fatalf("couldn't read fixture %s: %v", name, err)
	}
	return contents
}

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
		response := ReviewsResponse{
			Status: "Received", StatusCode: http.StatusOK, TotalCount: 1,
			Reviews: []Review{{Text: "An unfiltered English review"}},
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

func TestAuthorVersionFailover(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status int
		body   string
	}{
		{"older", http.StatusOK, `{"version":"BiblioReads v4.5.0","desc":"truncated","name":"stale"}`},
		{"prerelease", http.StatusOK, `{"version":"BiblioReads v4.5.1-rc.1","desc":"truncated"}`},
		{"missing", http.StatusOK, `{"desc":"truncated"}`},
		{"invalid", http.StatusOK, `{"version":"unknown","desc":"truncated"}`},
		{"old error", http.StatusNotFound, `{"version":"BiblioReads v4.5.0","status":"Error - Invalid Query"}`},
		{"unversioned error", http.StatusNotFound, `{"status":"Error - Invalid Query"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var hits atomic.Int32
			client := newFailoverClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if hits.Add(1) == 1 {
					w.WriteHeader(tt.status)
					_, _ = w.Write([]byte(tt.body))
					return
				}
				_, _ = w.Write([]byte(`{"version":"BiblioReads v4.5.1","status":"Received","desc":"complete"}`))
			}))
			for range 2 {
				response, err := client.Author(context.Background(), "1")
				if err != nil || response.Desc != "complete" || response.Name != "" {
					t.Fatalf("expected complete biography from compatible instance, got %+v, %v", response, err)
				}
			}
			if hits.Load() != 2 {
				t.Fatalf("expected failover and a cached compatible response, got %d requests", hits.Load())
			}
		})
	}
}

func TestAuthorVersionRequirements(t *testing.T) {
	for _, tt := range []struct {
		version string
		allowed bool
	}{
		{"", false}, {"invalid", false}, {"BiblioReads v4.5.0", false},
		{"BiblioReads v4.5.1-rc.1", false}, {"BiblioReads v4.5.1", true},
		{"v4.5.1", true}, {"BiblioReads v4.5.2", true}, {"BiblioReads v5.0.0", true},
	} {
		t.Run(tt.version, func(t *testing.T) {
			var hits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				if r.URL.Path == "/api/book-scraper" {
					_, _ = w.Write([]byte(validBookResponse))
					return
				}
				if err := json.NewEncoder(w).Encode(AuthorResponse{Version: tt.version, Desc: "biography"}); err != nil {
					t.Errorf("encode author response: %v", err)
				}
			}))
			defer server.Close()
			client := newTestClient(WithInstances([]string{server.URL}))
			for range 2 {
				response, err := client.Author(context.Background(), "1")
				if tt.allowed {
					if err != nil || response.Desc != "biography" {
						t.Fatalf("expected supported response, got %+v, %v", response, err)
					}
				} else if !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "v4.5.1") {
					t.Fatalf("expected explicit version error, got %v", err)
				}
			}
			if hits.Load() != 1 {
				t.Fatalf("expected compatible responses or incompatibility to be remembered, got %d requests", hits.Load())
			}
			if _, err := client.Book(context.Background(), "1"); err != nil {
				t.Fatalf("author version restrictions must not disable book requests: %v", err)
			}
		})
	}
}

func TestAuthorVersionCacheExpiresAfterUpgrade(t *testing.T) {
	var upgraded atomic.Bool
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		if upgraded.Load() {
			_, _ = w.Write([]byte(`{"version":"BiblioReads v4.5.1","desc":"complete"}`))
		} else {
			_, _ = w.Write([]byte(`{"version":"BiblioReads v4.5.0","desc":"truncated"}`))
		}
	}))
	defer server.Close()
	client := NewClient(WithInstances([]string{server.URL}), WithRequestInterval(0), WithCacheTTL(0))
	if _, err := client.Author(context.Background(), "1"); !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "v4.5.1") {
		t.Fatalf("expected incompatible version error, got %v", err)
	}
	upgraded.Store(true)
	if _, err := client.RefreshAuthor(context.Background(), "1"); !errors.Is(err, ErrUnavailable) || hits.Load() != 1 {
		t.Fatalf("known incompatible instance must be skipped: %v, hits %d", err, hits.Load())
	}
	client.mu.Lock()
	cached := client.authorVersions[server.URL]
	if remaining := time.Until(cached.expiresAt); remaining < 23*time.Hour || remaining > 24*time.Hour {
		client.mu.Unlock()
		t.Fatalf("expected author version to be remembered for 24 hours, got %v", remaining)
	}
	cached.expiresAt = time.Now().Add(-time.Second)
	client.authorVersions[server.URL] = cached
	client.mu.Unlock()
	response, err := client.Author(context.Background(), "1")
	if err != nil || response.Desc != "complete" || hits.Load() != 2 {
		t.Fatalf("upgraded instance must be retried after expiry: %+v, %v, hits %d", response, err, hits.Load())
	}
}

func TestAuthorSkipsKnownIncompatibleInstances(t *testing.T) {
	var firstHits, secondHits, total atomic.Int32
	handler := func(hits *atomic.Int32) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			version := "BiblioReads v4.5.1"
			if total.Add(1) == 1 {
				version = "BiblioReads v4.5.0"
			}
			if err := json.NewEncoder(w).Encode(AuthorResponse{Version: version, Desc: "biography"}); err != nil {
				t.Errorf("encode author response: %v", err)
			}
		}
	}
	first := httptest.NewServer(handler(&firstHits))
	defer first.Close()
	second := httptest.NewServer(handler(&secondHits))
	defer second.Close()
	client := newTestClient(WithInstances([]string{first.URL, second.URL}), WithCacheTTL(0))
	for i := range 12 {
		id := fmt.Sprint(i + 1)
		var err error
		if i%2 == 0 {
			_, err = client.Author(context.Background(), id)
		} else {
			_, err = client.RefreshAuthor(context.Background(), id)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if total.Load() != 13 || (firstHits.Load() != 1 && secondHits.Load() != 1) {
		t.Fatalf("expected one incompatible attempt then direct compatible lookups: first=%d second=%d", firstHits.Load(), secondHits.Load())
	}
}

func TestAuthorPrefersKnownCompatibleInstance(t *testing.T) {
	var firstHits, secondHits atomic.Int32
	handler := func(hits *atomic.Int32) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			if err := json.NewEncoder(w).Encode(AuthorResponse{Version: "BiblioReads v4.5.1", Desc: "biography"}); err != nil {
				t.Errorf("encode author response: %v", err)
			}
		}
	}
	first := httptest.NewServer(handler(&firstHits))
	defer first.Close()
	second := httptest.NewServer(handler(&secondHits))
	defer second.Close()
	client := newTestClient(WithInstances([]string{first.URL, second.URL}), WithCacheTTL(0))
	for i := range 12 {
		if _, err := client.Author(context.Background(), fmt.Sprint(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	if firstHits.Load()+secondHits.Load() != 12 || (firstHits.Load() != 0 && secondHits.Load() != 0) {
		t.Fatalf("expected known compatible instance to be preferred: first=%d second=%d", firstHits.Load(), secondHits.Load())
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
			client := newTestClient(WithInstances(instances), WithCacheTTL(0))
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

		client := newTestClient(WithInstances([]string{first.URL, second.URL}), WithCacheTTL(0))
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

		client := newTestClient(WithInstances([]string{first.URL, second.URL}), WithCacheTTL(0))
		_, err := client.Book(context.Background(), "0")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if errors.Is(err, ErrUnavailable) {
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

		client := newTestClient(WithInstances([]string{first.URL, second.URL}), WithCacheTTL(0))
		if _, err := client.Book(context.Background(), "1"); !errors.Is(err, ErrUnavailable) {
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
		client := newTestClient(WithInstances([]string{first.URL, second.URL}), WithCacheTTL(0))
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
		client := newTestClient(WithInstances([]string{server.URL}))

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

		client := newTestClient(WithInstances([]string{bad.URL, good.URL}), WithCacheTTL(0))
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

		client := newTestClient(WithInstances([]string{limited.URL, other.URL}), WithCacheTTL(0))
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

		client := newTestClient(WithInstances([]string{flaky.URL}), WithCacheTTL(0))
		if _, err := client.Book(context.Background(), "1"); err == nil {
			t.Fatal("expected error on first request, got nil")
		}
		if _, err := client.Book(context.Background(), "1"); !errors.Is(err, ErrUnavailable) {
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

		client := newTestClient(WithInstances([]string{first.URL, second.URL}), WithCacheTTL(0))
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

		client := newTestClient(WithInstances([]string{first.URL, second.URL}), WithCacheTTL(0))
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

func newCountingServer(t *testing.T, status int, body string, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}
