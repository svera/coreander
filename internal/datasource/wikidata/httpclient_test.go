package wikidata

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRateLimitedHTTPClientWaitsForRetryAfter(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"searchinfo":[],"search":[]}`))
	}))
	defer server.Close()

	client := newRateLimitedHTTPClient()
	client.client = server.Client()

	start := time.Now()
	var payload map[string]any
	if err := client.getJSON(server.URL, &payload); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatalf("expected 2 requests, got %d", requests.Load())
	}
	if elapsed := time.Since(start); elapsed < time.Second {
		t.Fatalf("expected to wait at least 1s after 429, took %s", elapsed)
	}
}

func TestRateLimitedHTTPClientBlocksConcurrentRequests(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requests.Add(1)
		if count == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"entities":{}}`))
	}))
	defer server.Close()

	client := newRateLimitedHTTPClient()
	client.client = server.Client()

	start := time.Now()
	done := make(chan struct{}, 2)
	for range 2 {
		go func() {
			var payload map[string]any
			_ = client.getJSON(server.URL, &payload)
			done <- struct{}{}
		}()
	}
	<-done
	<-done
	if elapsed := time.Since(start); elapsed < time.Second {
		t.Fatalf("expected concurrent requests to honor shared wait, took %s", elapsed)
	}
}
