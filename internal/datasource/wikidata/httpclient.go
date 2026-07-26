package wikidata

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const defaultRetryAfter = 60 * time.Second

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
		for key, value := range wikidataAPIHeaders() {
			req.Header.Set(key, value)
		}

		resp, err := c.client.Do(req)
		if err != nil {
			return err
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			_ = resp.Body.Close()
			c.waitUntil(resp.Header.Get("Retry-After"))
			continue
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			return fmt.Errorf("request failed with status code %d: %s", resp.StatusCode, body)
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return err
		}
		return json.Unmarshal(body, dest)
	}
}

// waitUntil blocks until the shared pause deadline has passed. When retryAfterHeader
// is non-empty, the deadline is extended using the Retry-After header value.
func (c *rateLimitedHTTPClient) waitUntil(retryAfterHeader string) {
	c.mu.Lock()
	var rateLimitWait time.Duration
	if retryAfterHeader != "" {
		rateLimitWait = parseRetryAfter(retryAfterHeader)
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

func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return defaultRetryAfter
	}
	if seconds, err := strconv.Atoi(header); err == nil {
		if seconds <= 0 {
			return defaultRetryAfter
		}
		return time.Duration(seconds) * time.Second
	}
	if retryTime, err := http.ParseTime(header); err == nil {
		wait := time.Until(retryTime)
		if wait <= 0 {
			return defaultRetryAfter
		}
		return wait
	}
	return defaultRetryAfter
}

func wikidataAPIHeaders() map[string]string {
	return map[string]string{
		"User-Agent": "Mozilla/5.0 (compatible; coreander/1.0; +https://github.com/svera/coreander)",
	}
}
