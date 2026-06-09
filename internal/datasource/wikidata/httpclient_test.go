package wikidata

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryAfterDuration(t *testing.T) {
	t.Run("seconds", func(t *testing.T) {
		resp := &http.Response{Header: http.Header{"Retry-After": []string{"30"}}}
		if got := retryAfterDuration(resp); got != 30*time.Second {
			t.Fatalf("expected 30s, got %s", got)
		}
	})

	t.Run("http date", func(t *testing.T) {
		retryTime := time.Now().Add(45 * time.Second).UTC()
		resp := &http.Response{Header: http.Header{"Retry-After": []string{retryTime.Format(http.TimeFormat)}}}
		got := retryAfterDuration(resp)
		if got < 44*time.Second || got > 46*time.Second {
			t.Fatalf("expected about 45s, got %s", got)
		}
	})

	t.Run("missing header uses default", func(t *testing.T) {
		resp := &http.Response{Header: http.Header{}}
		if got := retryAfterDuration(resp); got != defaultRetryAfter {
			t.Fatalf("expected default %s, got %s", defaultRetryAfter, got)
		}
	})
}

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
