package biblioreads_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
)

func TestRequestsAreThrottledPerInstance(t *testing.T) {
	const interval = 200 * time.Millisecond
	var mu sync.Mutex
	arrivals := make([][]time.Time, 2)
	var instances []string
	for i := range arrivals {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			arrivals[i] = append(arrivals[i], time.Now())
			mu.Unlock()
			_, _ = w.Write([]byte(validBookResponse))
		}))
		t.Cleanup(server.Close)
		instances = append(instances, server.URL)
	}
	client := biblioreads.NewClient(biblioreads.WithInstances(instances),
		biblioreads.WithCacheTTL(0), biblioreads.WithRequestInterval(interval))
	var wg sync.WaitGroup
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := client.Book(context.Background(), "1"); err != nil {
				t.Errorf("Book: %v", err)
			}
		}()
	}
	wg.Wait()
	for i, times := range arrivals {
		if len(times) != 3 {
			t.Fatalf("instance %d: expected three requests, got %d", i, len(times))
		}
		sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
		for j := 1; j < len(times); j++ {
			if gap := times[j].Sub(times[j-1]); gap < interval-20*time.Millisecond {
				t.Errorf("instance %d: request gap %s is below %s", i, gap, interval)
			}
		}
	}
	gap := arrivals[0][0].Sub(arrivals[1][0])
	if gap.Abs() >= interval/2 {
		t.Errorf("independent instances should start concurrently, gap %s", gap)
	}
}

func TestThrottleWaitHonorsCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(validBookResponse))
	}))
	defer server.Close()
	client := biblioreads.NewClient(biblioreads.WithInstances([]string{server.URL}),
		biblioreads.WithCacheTTL(0), biblioreads.WithRequestInterval(time.Second))
	if _, err := client.Book(context.Background(), "1"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := client.Book(ctx, "2"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded during throttle wait, got %v", err)
	}
	if time.Since(start) >= 500*time.Millisecond {
		t.Fatal("cancellation did not interrupt the throttle wait")
	}
}
