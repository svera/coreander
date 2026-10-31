// Package httpjson contains helpers shared by the datasource clients that consume JSON HTTP APIs.
package httpjson

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	UserAgent = "Mozilla/5.0 (compatible; coreander/1.0; +https://github.com/svera/coreander)"

	// DefaultRetryAfter is used when a Retry-After header is missing or invalid.
	DefaultRetryAfter = 60 * time.Second
)

type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// Do sends req with the default headers and reads the whole response body.
func Do(client *http.Client, req *http.Request) (Response, error) {
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{}, err
	}
	return Response{StatusCode: resp.StatusCode, Header: resp.Header, Body: body}, nil
}

// Decode unmarshals the response body into dest, returning an error if the status code is not 200.
func (r Response) Decode(dest any) error {
	if r.StatusCode != http.StatusOK {
		return fmt.Errorf("request failed with status code %d: %s", r.StatusCode, r.Body)
	}
	return json.Unmarshal(r.Body, dest)
}

// ParseRetryAfter returns the wait time specified by a Retry-After header value, which can be
// either a number of seconds or an HTTP date. DefaultRetryAfter is returned if it can't be used.
func ParseRetryAfter(header string) time.Duration {
	if header == "" {
		return DefaultRetryAfter
	}
	if seconds, err := strconv.Atoi(header); err == nil {
		if seconds <= 0 {
			return DefaultRetryAfter
		}
		return time.Duration(seconds) * time.Second
	}
	if retryTime, err := http.ParseTime(header); err == nil {
		wait := time.Until(retryTime)
		if wait <= 0 {
			return DefaultRetryAfter
		}
		return wait
	}
	return DefaultRetryAfter
}
