package httpjson_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/svera/coreander/v5/internal/datasource/httpjson"
)

func TestParseRetryAfter(t *testing.T) {
	t.Run("seconds", func(t *testing.T) {
		if got := httpjson.ParseRetryAfter("30"); got != 30*time.Second {
			t.Fatalf("expected 30s, got %s", got)
		}
	})

	t.Run("http date", func(t *testing.T) {
		retryTime := time.Now().Add(45 * time.Second).UTC()
		got := httpjson.ParseRetryAfter(retryTime.Format(http.TimeFormat))
		if got < 44*time.Second || got > 46*time.Second {
			t.Fatalf("expected about 45s, got %s", got)
		}
	})

	t.Run("missing header uses default", func(t *testing.T) {
		if got := httpjson.ParseRetryAfter(""); got != httpjson.DefaultRetryAfter {
			t.Fatalf("expected default %s, got %s", httpjson.DefaultRetryAfter, got)
		}
	})

	t.Run("invalid header uses default", func(t *testing.T) {
		for _, header := range []string{"-5", "0", "soon"} {
			if got := httpjson.ParseRetryAfter(header); got != httpjson.DefaultRetryAfter {
				t.Fatalf("expected default %s for %q, got %s", httpjson.DefaultRetryAfter, header, got)
			}
		}
	})
}

func TestDo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != httpjson.UserAgent {
			t.Errorf("unexpected User-Agent %q", r.Header.Get("User-Agent"))
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("unexpected Accept %q", r.Header.Get("Accept"))
		}
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte(`{"a":1}`))
	}))
	defer server.Close()

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := httpjson.Do(server.Client(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusTeapot || resp.Header.Get("Retry-After") != "5" || string(resp.Body) != `{"a":1}` {
		t.Errorf("unexpected response %+v", resp)
	}
}

func TestDecode(t *testing.T) {
	var dest struct{ A int }
	if err := (httpjson.Response{StatusCode: http.StatusOK, Body: []byte(`{"a":1}`)}).Decode(&dest); err != nil || dest.A != 1 {
		t.Errorf("expected A=1 without error, got %d, %v", dest.A, err)
	}
	if err := (httpjson.Response{StatusCode: http.StatusNotFound, Body: []byte(`{}`)}).Decode(&dest); err == nil {
		t.Error("expected error on non-200 status")
	}
	if err := (httpjson.Response{StatusCode: http.StatusOK, Body: []byte(`<html>`)}).Decode(&dest); err == nil {
		t.Error("expected error on invalid JSON")
	}
}
