package versioncheck

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCanonicalize(t *testing.T) {
	tests := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"v5.0.1", "v5.0.1", true},
		{"5.0.1", "v5.0.1", true},
		{"v5.0.1-3-gabc1234", "v5.0.1", true},
		{"v5.0.1-dirty", "v5.0.1", true},
		{"unknown", "", false},
		{"", "", false},
		{"not-a-version", "", false},
	}

	for _, tc := range tests {
		got, ok := canonicalize(tc.in)
		if ok != tc.wantOK {
			t.Fatalf("canonicalize(%q) ok = %v, want %v", tc.in, ok, tc.wantOK)
		}
		if got != tc.want {
			t.Fatalf("canonicalize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIsOlder(t *testing.T) {
	tests := []struct {
		running string
		latest  string
		want    bool
	}{
		{"v5.0.0", "v5.0.1", true},
		{"v5.0.1", "v5.0.1", false},
		{"v5.0.2", "v5.0.1", false},
		{"v5.0.1-2-gabc", "v5.0.2", true},
		{"unknown", "v5.0.1", false},
		{"v5.0.1", "not-a-version", false},
	}

	for _, tc := range tests {
		if got := isOlder(tc.running, tc.latest); got != tc.want {
			t.Fatalf("isOlder(%q, %q) = %v, want %v", tc.running, tc.latest, got, tc.want)
		}
	}
}

func TestFetchLatestReleaseTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9"}`))
	}))
	defer server.Close()

	original := releaseAPIURL
	releaseAPIURL = server.URL
	t.Cleanup(func() { releaseAPIURL = original })

	tag, err := fetchLatestReleaseTag()
	if err != nil {
		t.Fatalf("fetchLatestReleaseTag: %v", err)
	}
	if tag != "v9.9.9" {
		t.Fatalf("tag = %q, want v9.9.9", tag)
	}
}

func TestDisplayVersion(t *testing.T) {
	if got := displayVersion(""); got != "unknown" {
		t.Fatalf("displayVersion(\"\") = %q, want unknown", got)
	}
}
