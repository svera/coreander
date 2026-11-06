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

func newTestClient(opts ...biblioreads.Option) *biblioreads.Client {
	return biblioreads.NewClient(append([]biblioreads.Option{biblioreads.WithRequestInterval(0)}, opts...)...)
}

func newFailoverClient(t *testing.T, handler http.Handler) *biblioreads.Client {
	t.Helper()
	first := httptest.NewServer(handler)
	t.Cleanup(first.Close)
	second := httptest.NewServer(handler)
	t.Cleanup(second.Close)
	return newTestClient(biblioreads.WithInstances([]string{first.URL, second.URL}))
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	contents, err := os.ReadFile("fixtures/" + name)
	if err != nil {
		t.Fatalf("couldn't read fixture %s: %v", name, err)
	}
	return contents
}

func captureRequest(t *testing.T, w http.ResponseWriter, r *http.Request, captured *capturedRequest) bool {
	t.Helper()
	captured.method = r.Method
	captured.path = r.URL.Path
	captured.contentType = r.Header.Get("Content-Type")
	if err := json.NewDecoder(r.Body).Decode(&captured.payload); err != nil {
		t.Errorf("couldn't decode request payload: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return false
	}
	return true
}

func newMockServer(t *testing.T, status int, fixture string, captured *capturedRequest) *httptest.Server {
	t.Helper()
	contents := readFixture(t, fixture)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !captureRequest(t, w, r, captured) {
			return
		}
		w.WriteHeader(status)
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
	client := newTestClient(biblioreads.WithInstances([]string{server.URL}), biblioreads.WithHTTPClient(server.Client()), biblioreads.WithCacheTTL(0))

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
		got.RatingCount != "2285" ||
		got.ID != 1 {
		t.Errorf("unexpected result: %+v", got)
	}
	if res.NumberOfResults != "1 results" || res.SearchType != "books" {
		t.Errorf("unexpected response metadata: %+v", res)
	}
}

func TestBook(t *testing.T) {
	var captured capturedRequest
	server := newMockServer(t, http.StatusOK, "book-scraper.json", &captured)
	client := newTestClient(biblioreads.WithInstances([]string{server.URL}), biblioreads.WithHTTPClient(server.Client()), biblioreads.WithCacheTTL(0))

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
			client := newTestClient(biblioreads.WithInstances([]string{server.URL}), biblioreads.WithHTTPClient(server.Client()), biblioreads.WithCacheTTL(0))

			res, err := client.Author(context.Background(), tt.id)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertRequest(t, captured, "/api/author/info", map[string]any{"queryURL": tt.url})

			fixture := readFixture(t, "author-info.json")
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
			client := newTestClient(biblioreads.WithInstances([]string{server.URL}))

			for range 2 {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
				_, err := client.Author(ctx, "151155")
				cancel()
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if errors.Is(err, biblioreads.ErrUnavailable) != tt.unavailable && !errors.Is(err, context.DeadlineExceeded) {
					t.Errorf("expected unavailable=%v, got %v", tt.unavailable, err)
				}
			}
			wantHits := int32(2)
			if tt.unavailable {
				wantHits = 1
			}
			if hits.Load() != wantHits {
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
			client := newTestClient(biblioreads.WithInstances([]string{server.URL}))
			for range 2 {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
				_, err := client.Reviews(ctx, "work", 1, "", "")
				cancel()
				if !errors.Is(err, biblioreads.ErrUnavailable) && !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("expected unavailable error, got %v", err)
				}
			}
			if hits.Load() != 1 {
				t.Fatal("invalid responses must trigger cooldown")
			}
		})
	}
}

func TestReviewsLanguageVersionCompatibility(t *testing.T) {
	for _, tt := range []struct {
		version   string
		supported bool
	}{
		{"", false},
		{"BiblioReads v4.3.1", false},
		{"BiblioReads v4.4.0-beta.1", false},
		{"unknown", false},
		{"BiblioReads v4.4.0", true},
		{"BiblioReads v4.5.0", true},
	} {
		t.Run(tt.version, func(t *testing.T) {
			var hits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				if err := json.NewEncoder(w).Encode(biblioreads.ReviewsResponse{
					Version: tt.version, Status: "Received", StatusCode: http.StatusOK, TotalCount: 1,
					Reviews: []biblioreads.Review{{Text: "Review"}},
				}); err != nil {
					t.Errorf("encode response: %v", err)
				}
			}))
			defer server.Close()
			client := newTestClient(biblioreads.WithInstances([]string{server.URL}))
			for range 2 {
				_, err := client.Reviews(context.Background(), "work", 1, "", "es")
				if tt.supported && err != nil {
					t.Fatalf("expected supported language filter, got %v", err)
				}
				if !tt.supported && !errors.Is(err, biblioreads.ErrUnavailable) {
					t.Fatalf("expected unsupported instance to fail, got %v", err)
				}
			}
			wantHits := int32(1)
			if hits.Load() != wantHits {
				t.Fatalf("expected %d requests, got %d", wantHits, hits.Load())
			}
		})
	}
}

func TestReviews(t *testing.T) {
	t.Run("fractional ratings", func(t *testing.T) {
		var hits atomic.Int32
		server := newCountingServer(t, http.StatusOK, `{"status":"Received","statusCode":200,"totalCount":4,"reviews":[{"rating":0.25},{"rating":3.5},{"rating":4.75},{"rating":5}]}`, &hits)
		client := newTestClient(biblioreads.WithInstances([]string{server.URL}))
		response, err := client.Reviews(context.Background(), "work", 1, "", "")
		if err != nil {
			t.Fatal(err)
		}
		want := []float64{0.25, 3.5, 4.75, 5}
		if len(response.Reviews) != len(want) {
			t.Fatalf("expected %d reviews, got %d", len(want), len(response.Reviews))
		}
		for i, rating := range want {
			if response.Reviews[i].Rating != rating {
				t.Errorf("review %d: expected rating %g, got %g", i, rating, response.Reviews[i].Rating)
			}
		}
	})

	t.Run("first page", func(t *testing.T) {
		var captured capturedRequest
		server := newMockServer(t, http.StatusOK, "reviews-scraper.json", &captured)
		client := newTestClient(biblioreads.WithInstances([]string{server.URL}), biblioreads.WithHTTPClient(server.Client()), biblioreads.WithCacheTTL(0))

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
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !captureRequest(t, w, r, &captured) {
				return
			}
			_, _ = w.Write([]byte(`{"version":"BiblioReads v4.4.0","status":"Received","statusCode":200,"totalCount":1,"reviews":[{"text":"Una reseña"}]}`))
		}))
		defer server.Close()
		client := newTestClient(biblioreads.WithInstances([]string{server.URL}), biblioreads.WithHTTPClient(server.Client()), biblioreads.WithCacheTTL(0))

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
	var hits atomic.Int32
	server := newCountingServer(t, http.StatusNotFound, `{"status":"Error - Invalid Query"}`, &hits)
	client := newTestClient(biblioreads.WithInstances([]string{server.URL}), biblioreads.WithHTTPClient(server.Client()), biblioreads.WithCacheTTL(0))

	if _, err := client.Book(context.Background(), "0"); err == nil {
		t.Error("expected error on non-200 response, got nil")
	}
}

func TestInvalidSearchQuery(t *testing.T) {
	for _, tt := range []struct {
		name        string
		status      int
		body        string
		unavailable bool
	}{
		{"invalid query", http.StatusNotFound, `{"status":"Error - Invalid Query","version":"BiblioReads v4.4.0"}`, true},
		{"other not found", http.StatusNotFound, `{"status":"Error - Book not found"}`, false},
		{"other client error", http.StatusBadRequest, `{"status":"Error - Invalid Query"}`, false},
		{"server error", http.StatusServiceUnavailable, `{"status":"Error - Invalid Query"}`, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var hits atomic.Int32
			server := newCountingServer(t, tt.status, tt.body, &hits)
			client := newTestClient(biblioreads.WithInstances([]string{server.URL}))
			_, err := client.SearchBooks(context.Background(), "El economista camuflado Tim Harford")
			if err == nil || errors.Is(err, biblioreads.ErrUnavailable) != tt.unavailable {
				t.Fatalf("unexpected search error classification: %v", err)
			}
			if errors.Is(err, biblioreads.ErrNotFound) != (tt.status == http.StatusNotFound) {
				t.Fatalf("HTTP 404 classification must survive failover wrapping: %v", err)
			}
		})
	}
}

func TestNonJSONNotFoundClassification(t *testing.T) {
	var hits atomic.Int32
	server := newCountingServer(t, http.StatusNotFound, "<html>Not found</html>", &hits)
	client := newTestClient(biblioreads.WithInstances([]string{server.URL}))
	_, err := client.SearchBooks(context.Background(), "book")
	if !errors.Is(err, biblioreads.ErrNotFound) || !errors.Is(err, biblioreads.ErrUnavailable) {
		t.Fatalf("non-JSON 404 must retain its HTTP status classification: %v", err)
	}
}

func TestSearchInvalidQueryFailsOverWithoutChangingQuery(t *testing.T) {
	var hits atomic.Int32
	fixture := readFixture(t, "search-books.json")
	client := newFailoverClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var captured capturedRequest
		if !captureRequest(t, w, r, &captured) {
			return
		}
		assertRequest(t, captured, "/api/search/books", map[string]any{
			"queryURL": "https://www.goodreads.com/search?q=El%20economista%20camuflado%20Tim%20Harford",
		})
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"status":"Error - Invalid Query","version":"BiblioReads v4.4.0"}`))
			return
		}
		_, _ = w.Write(fixture)
	}))
	response, err := client.SearchBooks(context.Background(), "El economista camuflado Tim Harford")
	if err != nil || len(response.Result) == 0 || hits.Load() != 2 {
		t.Fatalf("expected same-query recovery on another instance, got %+v, %v, requests=%d", response, err, hits.Load())
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
