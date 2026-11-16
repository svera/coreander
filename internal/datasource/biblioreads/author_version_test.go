package biblioreads

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

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
