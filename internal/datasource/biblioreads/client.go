// Package biblioreads implements a client for the BiblioReads API
// (https://github.com/nesaku/biblioreads), an alternative Goodreads front-end.
package biblioreads

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/svera/coreander/v5/internal/datasource/httpjson"
	"golang.org/x/mod/semver"
)

const (
	goodreadsBaseURL = "https://www.goodreads.com"

	searchBooksPath    = "/api/search/books"
	bookScraperPath    = "/api/book-scraper"
	authorInfoPath     = "/api/author/info"
	reviewsScraperPath = "/api/reviews-scraper"
)

// DefaultInstances lists public BiblioReads instances.
var DefaultInstances = []string{
	"https://biblioreads.eu.org",
	"https://biblioreads.mooo.com",
	"https://biblioreads.lunar.icu",
	"https://biblioreads.ducks.party",
	"https://biblioreads.privacyredirect.com",
	"https://br.bloat.cat",
	"https://biblioreads.utilibre.org",
}

// ErrUnavailable is returned when no instance could serve a request (network errors, rate limits,
// non-JSON responses), as opposed to an instance answering with an error of its own.
var ErrUnavailable = errors.New("biblioreads: all instances failed")

type Client struct {
	instances  []string
	httpClient *http.Client
	cache      *responseCache

	mu              sync.Mutex
	cooldownUntil   map[string]time.Time
	nextRequest     map[string]time.Time
	requestInterval time.Duration
}

type options struct {
	instances       []string
	httpClient      *http.Client
	cacheTTL        time.Duration
	requestInterval time.Duration
}

type Option func(*options)

// WithRequestInterval sets the minimum spacing between request starts to each instance.
// Zero disables spacing; the default is one second.
func WithRequestInterval(interval time.Duration) Option {
	return func(o *options) {
		o.requestInterval = max(interval, 0)
	}
}

// WithInstances sets the BiblioReads instances to use. An empty list keeps DefaultInstances.
func WithInstances(instances []string) Option {
	return func(o *options) {
		if len(instances) > 0 {
			o.instances = instances
		}
	}
}

// WithHTTPClient sets the HTTP client used for requests. A nil client keeps the default one,
// which has a 15 seconds timeout.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(o *options) {
		if httpClient != nil {
			o.httpClient = httpClient
		}
	}
}

// WithCacheTTL sets how long successful responses are cached. A zero or negative ttl disables caching.
func WithCacheTTL(ttl time.Duration) Option {
	return func(o *options) {
		o.cacheTTL = ttl
	}
}

// NewClient returns a client that spreads requests across instances, preferring
// instances ready to accept a request and randomizing ties. Requests to each
// instance are spaced at least one second apart by default, including failover
// attempts. Unavailable instances are skipped until their Retry-After time, or
// httpjson.DefaultRetryAfter, has passed. Successful responses are cached in memory for
// DefaultCacheTTL unless changed with WithCacheTTL.
func NewClient(opts ...Option) *Client {
	o := options{
		instances:       DefaultInstances,
		httpClient:      &http.Client{Timeout: 15 * time.Second},
		cacheTTL:        DefaultCacheTTL,
		requestInterval: time.Second,
	}
	for _, opt := range opts {
		opt(&o)
	}

	normalized := make([]string, len(o.instances))
	for i, instance := range o.instances {
		normalized[i] = strings.TrimRight(instance, "/")
	}

	c := &Client{
		instances:       normalized,
		httpClient:      o.httpClient,
		cooldownUntil:   make(map[string]time.Time),
		nextRequest:     make(map[string]time.Time),
		requestInterval: o.requestInterval,
	}
	if o.cacheTTL > 0 {
		c.cache = newResponseCache(o.cacheTTL)
	}
	return c
}

type queryURLPayload struct {
	QueryURL string `json:"queryURL"`
}

type reviewsPayload struct {
	ResourceID    string `json:"resourceID"`
	LegacyBookID  int    `json:"legacyBookID"`
	NextPageToken string `json:"nextPageToken,omitempty"`
	LanguageCode  string `json:"languageCode,omitempty"`
}

// SearchBooks searches Goodreads books matching query.
func (c *Client) SearchBooks(ctx context.Context, query string) (SearchBooksResponse, error) {
	var res SearchBooksResponse
	payload := queryURLPayload{
		QueryURL: goodreadsBaseURL + "/search?q=" + escapeQuery(query),
	}
	err := c.post(ctx, searchBooksPath, payload, &res)
	return res, err
}

// Book returns the details of the Goodreads book identified by id, which can be
// either a numeric ID ("12207444") or an ID with slug ("12207444-la-reina-dragon").
func (c *Client) Book(ctx context.Context, id string) (BookResponse, error) {
	var res BookResponse
	payload := queryURLPayload{
		QueryURL: goodreadsBaseURL + "/book/show/" + url.PathEscape(id),
	}
	err := c.post(ctx, bookScraperPath, payload, &res)
	return res, err
}

// Author returns the details of the Goodreads author identified by id, which can be
// either a numeric ID ("151155") or an ID with slug ("151155.Alice_Borchardt").
func (c *Client) Author(ctx context.Context, id string) (AuthorResponse, error) {
	var res AuthorResponse
	payload := queryURLPayload{
		QueryURL: goodreadsBaseURL + "/author/show/" + url.PathEscape(id),
	}
	err := c.post(ctx, authorInfoPath, payload, &res)
	return res, err
}

// Reviews returns a page of reviews for the work identified by resourceID and legacyBookID
// (both available in BookResponse). Pass an empty nextPageToken to get the first page.
// languageCode filters reviews by a two-letter ISO language code; empty means all languages.
func (c *Client) Reviews(ctx context.Context, resourceID string, legacyBookID int, nextPageToken, languageCode string) (ReviewsResponse, error) {
	var res ReviewsResponse
	payload := reviewsPayload{
		ResourceID:    resourceID,
		LegacyBookID:  legacyBookID,
		NextPageToken: nextPageToken,
		LanguageCode:  languageCode,
	}
	err := c.post(ctx, reviewsScraperPath, payload, &res)
	return res, err
}

// unavailableError means the instance can't serve requests right now and another one should be tried.
type unavailableError struct {
	endpoint   string
	retryAfter time.Duration
	reason     error
}

func (e *unavailableError) Error() string {
	return fmt.Sprintf("%s: %v", e.endpoint, e.reason)
}

func (e *unavailableError) Unwrap() error {
	return e.reason
}

func (c *Client) post(ctx context.Context, path string, payload, dest any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	cacheKey := path + "\x00" + string(body)
	if c.cache != nil {
		if cached, ok := c.cache.get(cacheKey); ok {
			return json.Unmarshal(cached, dest)
		}
	}

	var errs []error
	var incompleteResponse []byte
	tried := make(map[string]bool)
	for len(tried) < len(c.instances) {
		instance, err := c.acquireInstance(ctx, tried)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			errs = append(errs, err)
			break
		}
		tried[instance] = true
		endpoint := instance + path
		respBody, err := c.postToInstance(ctx, endpoint, body)
		if err == nil {
			var complete bool
			complete, err = decodeResponse(respBody, payload, dest)
			if err != nil {
				if path != bookScraperPath && path != reviewsScraperPath {
					return fmt.Errorf("biblioreads: error decoding response from %s: %w", endpoint, err)
				}
				err = &unavailableError{endpoint: endpoint, retryAfter: httpjson.DefaultRetryAfter, reason: err}
			} else if !complete {
				// Keep a fallback, preferring book details that include a score.
				book, isBook := dest.(*BookResponse)
				if incompleteResponse == nil || !isBook || book.Rating != "" {
					incompleteResponse = respBody
				}
				continue
			}
		}
		if err == nil {
			reviews, isReviews := dest.(*ReviewsResponse)
			if c.cache != nil && (!isReviews || len(reviews.Reviews) > 0) {
				c.cache.set(cacheKey, respBody)
			}
			return nil
		}
		var unavailable *unavailableError
		if !errors.As(err, &unavailable) {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		c.setCooldown(instance, unavailable.retryAfter)
		errs = append(errs, err)
	}
	if incompleteResponse != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := decodeResponse(incompleteResponse, payload, dest)
		return err
	}
	return fmt.Errorf("%w: %w", ErrUnavailable, errors.Join(errs...))
}

// decodeResponse reports whether a valid response is complete enough to stop failover.
func decodeResponse(body []byte, payload, dest any) (bool, error) {
	switch response := dest.(type) {
	case *BookResponse:
		*response = BookResponse{}
		if err := json.Unmarshal(body, response); err != nil {
			return false, err
		}
		if response.Status != "Received" || response.StatusCode != http.StatusOK {
			return false, fmt.Errorf("invalid book response: status %q, code %d", response.Status, response.StatusCode)
		}
		return response.Rating != "" && response.ResourceID != "" && response.LegacyBookID > 0, nil
	case *ReviewsResponse:
		*response = ReviewsResponse{}
		if err := json.Unmarshal(body, response); err != nil {
			return false, err
		}
		request, ok := payload.(reviewsPayload)
		if !ok {
			return false, fmt.Errorf("invalid reviews request payload %T", payload)
		}
		if request.LanguageCode != "" && !supportsReviewLanguage(response.Version) {
			return false, fmt.Errorf("review language filtering requires BiblioReads v4.4.0 or newer, got %q", response.Version)
		}
		if response.Status != "Received" || response.StatusCode != http.StatusOK || response.Reviews == nil ||
			(request.NextPageToken == "" && response.TotalCount > 0 && len(response.Reviews) == 0) {
			return false, fmt.Errorf("invalid reviews response: status %q, code %d, total %d, reviews %d",
				response.Status, response.StatusCode, response.TotalCount, len(response.Reviews))
		}
		return request.NextPageToken != "" || len(response.Reviews) > 0, nil
	default:
		return true, json.Unmarshal(body, dest)
	}
}

func supportsReviewLanguage(version string) bool {
	version = strings.TrimPrefix(version, "BiblioReads ")
	return semver.IsValid(version) && semver.Compare(version, "v4.4.0") >= 0
}

// acquireInstance rechecks availability after waiting, so cancellation does not
// reserve a future slot and concurrent requests cannot take the same slot.
func (c *Client) acquireInstance(ctx context.Context, tried map[string]bool) (string, error) {
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		c.mu.Lock()
		start := rand.IntN(len(c.instances))
		instance := ""
		now := time.Now()
		for i := range c.instances {
			candidate := c.instances[(start+i)%len(c.instances)]
			if !tried[candidate] && !now.Before(c.cooldownUntil[candidate]) &&
				(instance == "" || c.nextRequest[candidate].Before(c.nextRequest[instance])) {
				instance = candidate
			}
		}
		if instance == "" {
			c.mu.Unlock()
			return "", fmt.Errorf("%w: no untried instance is outside its cooldown", ErrUnavailable)
		}
		wait := time.Until(c.nextRequest[instance])
		if wait <= 0 {
			c.nextRequest[instance] = time.Now().Add(c.requestInterval)
			c.mu.Unlock()
			return instance, nil
		}
		c.mu.Unlock()
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *Client) setCooldown(instance string, d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cooldownUntil[instance] = time.Now().Add(d)
}

// postToInstance returns the body of a successful response.
func (c *Client) postToInstance(ctx context.Context, endpoint string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpjson.Do(c.httpClient, req)
	if err != nil {
		return nil, &unavailableError{endpoint: endpoint, retryAfter: httpjson.DefaultRetryAfter, reason: err}
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, &unavailableError{
			endpoint:   endpoint,
			retryAfter: httpjson.ParseRetryAfter(resp.Header.Get("Retry-After")),
			reason:     fmt.Errorf("status code %d", resp.StatusCode),
		}
	}
	if resp.StatusCode >= http.StatusInternalServerError {
		return nil, &unavailableError{
			endpoint: endpoint, retryAfter: httpjson.DefaultRetryAfter,
			reason: fmt.Errorf("status code %d: %s", resp.StatusCode, resp.Body),
		}
	}

	// BiblioReads always answers with JSON, even on errors (e.g. a 504 for a book that
	// doesn't exist), so anything else comes from a broken instance or a proxy in front of it.
	if !json.Valid(resp.Body) {
		return nil, &unavailableError{
			endpoint:   endpoint,
			retryAfter: httpjson.DefaultRetryAfter,
			reason:     fmt.Errorf("status code %d with non-JSON response", resp.StatusCode),
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("biblioreads: request to %s failed with status code %d: %s", endpoint, resp.StatusCode, resp.Body)
	}
	return resp.Body, nil
}

// escapeQuery escapes a search string using %20 for spaces, as Goodreads does.
func escapeQuery(query string) string {
	return strings.ReplaceAll(url.QueryEscape(query), "+", "%20")
}
