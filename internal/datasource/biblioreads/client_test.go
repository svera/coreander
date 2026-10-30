package biblioreads_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
)

type capturedRequest struct {
	method      string
	path        string
	contentType string
	payload     map[string]any
}

const validBookResponse = `{"status":"Received","statusCode":200,"rating":"3.92","resourceID":"work-id","legacyBookID":5898652}`

func newMockServer(t *testing.T, status int, fixture string, captured *capturedRequest) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.method = r.Method
		captured.path = r.URL.Path
		captured.contentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&captured.payload); err != nil {
			t.Errorf("couldn't decode request payload: %v", err)
		}
		w.WriteHeader(status)
		if fixture == "" {
			_, _ = w.Write([]byte(`{"status":"Error - Invalid Query"}`))
			return
		}
		contents, err := os.ReadFile("fixtures/" + fixture)
		if err != nil {
			t.Fatalf("couldn't read fixture %s: %v", fixture, err)
		}
		_, _ = w.Write(contents)
	}))
	t.Cleanup(server.Close)
	return server
}

func assertRequest(t *testing.T, captured capturedRequest, path string, payload map[string]any) {
	t.Helper()
	if captured.method != http.MethodPost {
		t.Errorf("expected POST, got %s", captured.method)
	}
	if captured.path != path {
		t.Errorf("expected path %s, got %s", path, captured.path)
	}
	if captured.contentType != "application/json" {
		t.Errorf("expected application/json content type, got %s", captured.contentType)
	}
	if len(captured.payload) != len(payload) {
		t.Errorf("expected payload %v, got %v", payload, captured.payload)
	}
	for key, value := range payload {
		if captured.payload[key] != value {
			t.Errorf("expected payload[%s] = %v, got %v", key, value, captured.payload[key])
		}
	}
}

func TestSearchBooks(t *testing.T) {
	var captured capturedRequest
	server := newMockServer(t, http.StatusOK, "search-books.json", &captured)
	client := biblioreads.NewClient(biblioreads.WithInstances([]string{server.URL}), biblioreads.WithHTTPClient(server.Client()), biblioreads.WithCacheTTL(0))

	res, err := client.SearchBooks(context.Background(), "La Reina Dragón Alice Borchardt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertRequest(t, captured, "/api/search/books", map[string]any{
		"queryURL": "https://www.goodreads.com/search?q=La%20Reina%20Drag%C3%B3n%20Alice%20Borchardt",
	})

	if len(res.Result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(res.Result))
	}
	got := res.Result[0]
	if got.Title != "La Reina Dragon  (La leyenda de Guinevere, #1)" ||
		got.BookURL != "/book/show/12207444-la-reina-dragon" ||
		got.Author != "Alice Borchardt" ||
		got.Rating != "3.82" ||
		got.ID != 1 {
		t.Errorf("unexpected result: %+v", got)
	}
	if res.NumberOfResults != "1 results" || res.SearchType != "books" {
		t.Errorf("unexpected response metadata: %+v", res)
	}
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
		{"missing score", http.StatusOK, `{"status":"Received","statusCode":200,"resourceID":"work-id","legacyBookID":5898652}`},
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
			first := httptest.NewServer(handler)
			defer first.Close()
			second := httptest.NewServer(handler)
			defer second.Close()
			client := biblioreads.NewClient(biblioreads.WithInstances([]string{first.URL, second.URL}))
			for range 2 {
				book, err := client.Book(context.Background(), "5898652-la-cinta-roja")
				if err != nil || book.Rating != "3.92" || book.ResourceID != "work-id" || book.LegacyBookID != 5898652 {
					t.Fatalf("expected complete book after failover, got %+v, %v", book, err)
				}
			}
			if hits.Load() != 2 {
				t.Fatalf("expected recovered book to be cached, got %d requests", hits.Load())
			}
		})
	}
}

func TestIncompleteBookResponsesAreNotCached(t *testing.T) {
	var hits atomic.Int32
	server := newCountingServer(t, http.StatusOK, `{"status":"Received","statusCode":200,"rating":"3.92"}`, &hits)
	client := biblioreads.NewClient(biblioreads.WithInstances([]string{server.URL}))
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

func TestBook(t *testing.T) {
	var captured capturedRequest
	server := newMockServer(t, http.StatusOK, "book-scraper.json", &captured)
	client := biblioreads.NewClient(biblioreads.WithInstances([]string{server.URL}), biblioreads.WithHTTPClient(server.Client()), biblioreads.WithCacheTTL(0))

	res, err := client.Book(context.Background(), "12207444-la-reina-dragon")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertRequest(t, captured, "/api/book-scraper", map[string]any{
		"queryURL": "https://www.goodreads.com/book/show/12207444-la-reina-dragon",
	})

	if res.LegacyBookID != 12207444 {
		t.Errorf("expected legacy book ID 12207444, got %d", res.LegacyBookID)
	}
	if res.ResourceID != "kca://work/amzn1.gr.work.v1.dbcQjzlbvXR8RBZBdd8J6g" {
		t.Errorf("unexpected resource ID %s", res.ResourceID)
	}
	if res.Title != "La Reina Dragon" || res.Series != "Tales of Guinevere" || res.SeriesPosition != "1" {
		t.Errorf("unexpected book data: %+v", res)
	}
	if len(res.Author) != 1 || res.Author[0].Name != "Alice Borchardt" || res.Author[0].Role != "Author" {
		t.Errorf("unexpected authors: %+v", res.Author)
	}
	if len(res.Genres) != 10 || res.Genres[0] != "Fantasy" {
		t.Errorf("unexpected genres: %v", res.Genres)
	}
	if res.ISBN13 != "9788448033187" || res.Language != "Spanish; Castilian" {
		t.Errorf("unexpected book data: %+v", res)
	}
	if res.ReviewBreakdown.Rating5 != "726" {
		t.Errorf("unexpected review breakdown: %+v", res.ReviewBreakdown)
	}
}

func TestAuthor(t *testing.T) {
	for _, tt := range []struct {
		name string
		id   string
		url  string
	}{
		{"numeric ID", "151155", "https://www.goodreads.com/author/show/151155"},
		{"ID with slug", "151155.Alice_Borchardt", "https://www.goodreads.com/author/show/151155.Alice_Borchardt"},
		{"escaped ID", "151155.Alice/Borchardt?#", "https://www.goodreads.com/author/show/151155.Alice%2FBorchardt%3F%23"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var captured capturedRequest
			server := newMockServer(t, http.StatusOK, "author-info.json", &captured)
			client := biblioreads.NewClient(biblioreads.WithInstances([]string{server.URL}), biblioreads.WithHTTPClient(server.Client()), biblioreads.WithCacheTTL(0))

			res, err := client.Author(context.Background(), tt.id)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertRequest(t, captured, "/api/author/info", map[string]any{"queryURL": tt.url})

			fixture, err := os.ReadFile("fixtures/author-info.json")
			if err != nil {
				t.Fatalf("couldn't read author fixture: %v", err)
			}
			encoded, err := json.Marshal(res)
			if err != nil {
				t.Fatalf("couldn't encode author response: %v", err)
			}
			var want, got map[string]any
			if err := json.Unmarshal(fixture, &want); err != nil {
				t.Fatalf("couldn't decode author fixture: %v", err)
			}
			if err := json.Unmarshal(encoded, &got); err != nil {
				t.Fatalf("couldn't decode encoded author response: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("author response did not preserve all fixture fields:\ngot: %+v\nwant: %+v", got, want)
			}
		})
	}
}

func TestAuthorErrors(t *testing.T) {
	for _, tt := range []struct {
		name        string
		status      int
		body        string
		unavailable bool
	}{
		{"API error", http.StatusNotFound, `{"status":"Error - Author not found"}`, false},
		{"non-JSON response", http.StatusOK, "<html>Bad Gateway</html>", true},
		{"invalid field type", http.StatusOK, `{"books":"invalid"}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var hits atomic.Int32
			server := newCountingServer(t, tt.status, tt.body, &hits)
			client := biblioreads.NewClient(biblioreads.WithInstances([]string{server.URL}))

			for range 2 {
				_, err := client.Author(context.Background(), "151155")
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if errors.Is(err, biblioreads.ErrUnavailable) != tt.unavailable {
					t.Errorf("expected unavailable=%v, got %v", tt.unavailable, err)
				}
			}
			if hits.Load() != 2 {
				t.Errorf("expected errors not to be cached, got %d requests", hits.Load())
			}
		})
	}
}

func TestReviewsInvalidResponses(t *testing.T) {
	for _, body := range []string{
		`{}`,
		`{"status":"Error - Invalid Query","statusCode":500}`,
		`{"status":"Received","statusCode":200,"totalCount":90,"reviews":[]}`,
		`{"status":"Received","statusCode":200,"totalCount":0}`,
		`{"status":"Received","statusCode":200,"reviews":"invalid"}`,
	} {
		t.Run(body, func(t *testing.T) {
			var hits atomic.Int32
			server := newCountingServer(t, http.StatusOK, body, &hits)
			client := biblioreads.NewClient(biblioreads.WithInstances([]string{server.URL}))
			for range 2 {
				if _, err := client.Reviews(context.Background(), "work", 1, "", ""); !errors.Is(err, biblioreads.ErrUnavailable) {
					t.Fatalf("expected unavailable error, got %v", err)
				}
			}
			if hits.Load() != 2 {
				t.Fatal("invalid responses must not be cached")
			}
		})
	}
}

func TestReviewsFailover(t *testing.T) {
	fixture, err := os.ReadFile("fixtures/reviews-scraper.json")
	if err != nil {
		t.Fatal(err)
	}
	var hits atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		_, _ = w.Write(fixture)
	})
	first := httptest.NewServer(handler)
	defer first.Close()
	second := httptest.NewServer(handler)
	defer second.Close()
	client := biblioreads.NewClient(biblioreads.WithInstances([]string{first.URL, second.URL}))
	response, err := client.Reviews(context.Background(), "work", 1, "", "")
	if err != nil || len(response.Reviews) == 0 || hits.Load() != 2 {
		t.Fatalf("expected failover to working instance, got %d requests, %+v, %v", hits.Load(), response, err)
	}
}

func TestReviewsEmptyResponseRecovery(t *testing.T) {
	fixture, err := os.ReadFile("fixtures/reviews-scraper.json")
	if err != nil {
		t.Fatal(err)
	}
	var hits atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"status":"Received","statusCode":200,"totalCount":0,"reviews":[]}`))
			return
		}
		_, _ = w.Write(fixture)
	})
	first := httptest.NewServer(handler)
	defer first.Close()
	second := httptest.NewServer(handler)
	defer second.Close()
	client := biblioreads.NewClient(biblioreads.WithInstances([]string{first.URL, second.URL}))
	for range 2 {
		reviews, err := client.Reviews(context.Background(), "work", 1, "", "")
		if err != nil || len(reviews.Reviews) == 0 {
			t.Fatalf("expected reviews after empty first response, got %+v, %v", reviews, err)
		}
	}
	if hits.Load() != 2 {
		t.Fatalf("expected non-empty reviews to be cached, got %d requests", hits.Load())
	}
}

func TestReviewsEmptyResponsesAreNotCached(t *testing.T) {
	var hits atomic.Int32
	server := newCountingServer(t, http.StatusOK, `{"status":"Received","statusCode":200,"totalCount":0,"reviews":[]}`, &hits)
	client := biblioreads.NewClient(biblioreads.WithInstances([]string{server.URL}))
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
			Status: "Received", StatusCode: http.StatusOK, TotalCount: 1,
			Reviews: []biblioreads.Review{{Text: payload.LanguageCode}},
		})
	}))
	defer server.Close()
	client := biblioreads.NewClient(biblioreads.WithInstances([]string{server.URL}))
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

func TestReviews(t *testing.T) {
	t.Run("first page", func(t *testing.T) {
		var captured capturedRequest
		server := newMockServer(t, http.StatusOK, "reviews-scraper.json", &captured)
		client := biblioreads.NewClient(biblioreads.WithInstances([]string{server.URL}), biblioreads.WithHTTPClient(server.Client()), biblioreads.WithCacheTTL(0))

		res, err := client.Reviews(context.Background(), "kca://work/amzn1.gr.work.v1.dbcQjzlbvXR8RBZBdd8J6g", 12207444, "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		assertRequest(t, captured, "/api/reviews-scraper", map[string]any{
			"resourceID":   "kca://work/amzn1.gr.work.v1.dbcQjzlbvXR8RBZBdd8J6g",
			"legacyBookID": float64(12207444),
		})

		if res.TotalCount != 115 || res.NextPageToken != "NTgsMTcwNTAxNjc4NjQzNA" {
			t.Errorf("unexpected response metadata: %+v", res)
		}
		if len(res.Reviews) == 0 {
			t.Fatal("expected reviews, got none")
		}
		got := res.Reviews[0]
		if got.Rating != 5 || got.Shelf != "read" || got.LikeCount != 18 || got.RecommendFor != nil {
			t.Errorf("unexpected review: %+v", got)
		}
		if got.Reviewer.ID != 1259454 || got.Reviewer.Name != "Shawna Astley" || got.Reviewer.ReviewCount != 42 {
			t.Errorf("unexpected reviewer: %+v", got.Reviewer)
		}
	})

	t.Run("next page", func(t *testing.T) {
		var captured capturedRequest
		server := newMockServer(t, http.StatusOK, "reviews-scraper.json", &captured)
		client := biblioreads.NewClient(biblioreads.WithInstances([]string{server.URL}), biblioreads.WithHTTPClient(server.Client()), biblioreads.WithCacheTTL(0))

		if _, err := client.Reviews(context.Background(), "kca://work/abc", 1, "token", "es"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		assertRequest(t, captured, "/api/reviews-scraper", map[string]any{
			"resourceID":    "kca://work/abc",
			"legacyBookID":  float64(1),
			"nextPageToken": "token",
			"languageCode":  "es",
		})
	})
}

func TestErrorStatus(t *testing.T) {
	var captured capturedRequest
	server := newMockServer(t, http.StatusNotFound, "", &captured)
	client := biblioreads.NewClient(biblioreads.WithInstances([]string{server.URL}), biblioreads.WithHTTPClient(server.Client()), biblioreads.WithCacheTTL(0))

	if _, err := client.Book(context.Background(), "0"); err == nil {
		t.Error("expected error on non-200 response, got nil")
	}
}

func newCountingServer(t *testing.T, status int, body string, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
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
			client := biblioreads.NewClient(biblioreads.WithInstances(instances), biblioreads.WithCacheTTL(0))
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

		client := biblioreads.NewClient(biblioreads.WithInstances([]string{first.URL, second.URL}), biblioreads.WithCacheTTL(0))
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

		client := biblioreads.NewClient(biblioreads.WithInstances([]string{first.URL, second.URL}), biblioreads.WithCacheTTL(0))
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

		client := biblioreads.NewClient(biblioreads.WithInstances([]string{first.URL, second.URL}), biblioreads.WithCacheTTL(0))
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
		client := biblioreads.NewClient(biblioreads.WithInstances([]string{first.URL, second.URL}), biblioreads.WithCacheTTL(0))
		if _, err := client.Book(ctx, "1"); err == nil {
			t.Fatal("expected error, got nil")
		}
		if hits.Load() != 0 {
			t.Errorf("expected no requests, got %d", hits.Load())
		}
	})
}

func TestCooldown(t *testing.T) {
	t.Run("failing instance is skipped on later requests", func(t *testing.T) {
		var badHits, goodHits atomic.Int32
		bad := newCountingServer(t, http.StatusBadGateway, "<html>Bad Gateway</html>", &badHits)
		good := newCountingServer(t, http.StatusOK, validBookResponse, &goodHits)

		client := biblioreads.NewClient(biblioreads.WithInstances([]string{bad.URL, good.URL}), biblioreads.WithCacheTTL(0))
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

		client := biblioreads.NewClient(biblioreads.WithInstances([]string{limited.URL, other.URL}), biblioreads.WithCacheTTL(0))
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

	t.Run("instance is probed when all are cooling down", func(t *testing.T) {
		var hits atomic.Int32
		flaky := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if hits.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(validBookResponse))
		}))
		t.Cleanup(flaky.Close)

		client := biblioreads.NewClient(biblioreads.WithInstances([]string{flaky.URL}), biblioreads.WithCacheTTL(0))
		if _, err := client.Book(context.Background(), "1"); err == nil {
			t.Fatal("expected error on first request, got nil")
		}
		if _, err := client.Book(context.Background(), "1"); err != nil {
			t.Fatalf("expected cooling down instance to be probed, got %v", err)
		}
	})

	t.Run("JSON client error responses don't trigger cooldown", func(t *testing.T) {
		var firstHits, secondHits atomic.Int32
		notFound := `{"status":"Error - Book not found"}`
		first := newCountingServer(t, http.StatusNotFound, notFound, &firstHits)
		second := newCountingServer(t, http.StatusNotFound, notFound, &secondHits)

		client := biblioreads.NewClient(biblioreads.WithInstances([]string{first.URL, second.URL}), biblioreads.WithCacheTTL(0))
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

		client := biblioreads.NewClient(biblioreads.WithInstances([]string{first.URL, second.URL}), biblioreads.WithCacheTTL(0))
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

func TestCache(t *testing.T) {
	t.Run("successful responses are cached per request", func(t *testing.T) {
		var hits atomic.Int32
		server := newCountingServer(t, http.StatusOK, `{"status":"Received","statusCode":200,"rating":"3.92","resourceID":"work-id","legacyBookID":5898652,"title":"La Reina Dragon","genres":["Fantasy"],"totalCount":1,"reviews":[{"id":"review-1"}]}`, &hits)
		client := biblioreads.NewClient(biblioreads.WithInstances([]string{server.URL}))

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
		client := biblioreads.NewClient(biblioreads.WithInstances([]string{server.URL}), biblioreads.WithCacheTTL(100*time.Millisecond))

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

	t.Run("errors are not cached", func(t *testing.T) {
		var hits atomic.Int32
		server := newCountingServer(t, http.StatusGatewayTimeout, `{"status":"Error - Book not found"}`, &hits)
		client := biblioreads.NewClient(biblioreads.WithInstances([]string{server.URL}))

		for range 3 {
			if _, err := client.Book(context.Background(), "0"); err == nil {
				t.Fatal("expected error, got nil")
			}
		}
		if hits.Load() != 3 {
			t.Errorf("expected every failed request to reach the server, got %d requests", hits.Load())
		}
	})

	t.Run("caching can be disabled", func(t *testing.T) {
		var hits atomic.Int32
		server := newCountingServer(t, http.StatusOK, validBookResponse, &hits)
		client := biblioreads.NewClient(biblioreads.WithInstances([]string{server.URL}), biblioreads.WithCacheTTL(0))

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
