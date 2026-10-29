package wikidata

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/svera/coreander/v5/internal/datasource/httpjson"
)

var apiHTTPClient = newRateLimitedHTTPClient()

type rateLimitedHTTPClient struct {
	mu       sync.Mutex
	resumeAt time.Time
	client   *http.Client
}

func newRateLimitedHTTPClient() *rateLimitedHTTPClient {
	return &rateLimitedHTTPClient{
		client: &http.Client{},
	}
}

func (c *rateLimitedHTTPClient) getJSON(url string, dest any) error {
	for {
		c.waitUntil("")

		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return err
		}

		resp, err := httpjson.Do(c.client, req)
		if err != nil {
			return err
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			c.waitUntil(resp.Header.Get("Retry-After"))
			continue
		}

		return resp.Decode(dest)
	}
}

// waitUntil blocks until the shared pause deadline has passed. When retryAfterHeader
// is non-empty, the deadline is extended using the Retry-After header value.
func (c *rateLimitedHTTPClient) waitUntil(retryAfterHeader string) {
	c.mu.Lock()
	var rateLimitWait time.Duration
	if retryAfterHeader != "" {
		rateLimitWait = httpjson.ParseRetryAfter(retryAfterHeader)
		resumeAt := time.Now().Add(rateLimitWait)
		if resumeAt.After(c.resumeAt) {
			c.resumeAt = resumeAt
		}
	}
	wait := time.Until(c.resumeAt)
	c.mu.Unlock()
	if retryAfterHeader != "" {
		log.Printf("Wikidata rate limit reached, waiting %s before retrying", rateLimitWait)
	}
	if wait > 0 {
		time.Sleep(wait)
	}
}
