package main

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

func TestReviewSourceInstances(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  string
		want []string
	}{
		{name: "defaults"},
		{name: "single", args: []string{"--review-source-instances=https://br.example.org"}, want: []string{"https://br.example.org"}},
		{name: "comma separated", args: []string{"--review-source-instances=https://br.example.org,http://localhost:8080/"}, want: []string{"https://br.example.org", "http://localhost:8080/"}},
		{name: "repeated", args: []string{"--review-source-instances=https://br.example.org", "--review-source-instances=https://br2.example.org"}, want: []string{"https://br.example.org", "https://br2.example.org"}},
		{name: "environment", env: "https://br.example.org,https://br2.example.org", want: []string{"https://br.example.org", "https://br2.example.org"}},
		{name: "flag overrides environment", env: "https://env.example.org", args: []string{"--review-source-instances=https://cli.example.org"}, want: []string{"https://cli.example.org"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("REVIEW_SOURCE_INSTANCES", tt.env)
			var input CLIInput
			parser := newCLIParser(t, &input)
			if _, err := parser.Parse(append(tt.args, t.TempDir())); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(input.ReviewSourceInstances, tt.want) {
				t.Errorf("instances = %v, want %v", input.ReviewSourceInstances, tt.want)
			}
		})
	}
}

func TestReviewSourceInstancesRejectInvalidURLs(t *testing.T) {
	for _, instance := range []string{"br.example.org", "/relative", "ftp://br.example.org", "https://", "https://bad host", "https://br.example.org,invalid"} {
		t.Run(instance, func(t *testing.T) {
			var input CLIInput
			parser := newCLIParser(t, &input)
			_, err := parser.Parse([]string{"--review-source-instances=" + instance, t.TempDir()})
			if err == nil || !strings.Contains(err.Error(), "review-source-instances") {
				t.Fatalf("expected review-source-instances validation error, got %v", err)
			}
		})
	}
}

func TestReviewSourceSettings(t *testing.T) {
	for _, tt := range []struct {
		name     string
		args     []string
		disabled string
		ttl      string
		wantOff  bool
		wantTTL  int
	}{
		{name: "defaults", wantTTL: 86400},
		{name: "flags", args: []string{"--disable-reviews", "--review-source-cache-ttl=120"}, wantOff: true, wantTTL: 120},
		{name: "environment", disabled: "true", ttl: "300", wantOff: true, wantTTL: 300},
		{name: "flags override environment", disabled: "true", ttl: "300", args: []string{"--disable-reviews=false", "--review-source-cache-ttl=0"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DISABLE_REVIEWS", tt.disabled)
			t.Setenv("REVIEW_SOURCE_CACHE_TTL", tt.ttl)
			for key, value := range map[string]string{
				"DISABLE_REVIEWS": tt.disabled, "REVIEW_SOURCE_CACHE_TTL": tt.ttl,
			} {
				if value == "" {
					if err := os.Unsetenv(key); err != nil {
						t.Fatal(err)
					}
				}
			}
			var input CLIInput
			parser := newCLIParser(t, &input)
			if _, err := parser.Parse(append(tt.args, t.TempDir())); err != nil {
				t.Fatal(err)
			}
			if input.DisableReviews != tt.wantOff || input.ReviewSourceCacheTTL != tt.wantTTL {
				t.Fatalf("disabled/TTL = %v/%d, want %v/%d", input.DisableReviews, input.ReviewSourceCacheTTL, tt.wantOff, tt.wantTTL)
			}
		})
	}
}

func TestReviewSourceCacheTTLRejectsNegative(t *testing.T) {
	var input CLIInput
	parser := newCLIParser(t, &input)
	_, err := parser.Parse([]string{"--review-source-cache-ttl=-1", t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "review-source-cache-ttl") {
		t.Fatalf("expected review-source-cache-ttl validation error, got %v", err)
	}
}

func newCLIParser(t *testing.T, input *CLIInput) *kong.Kong {
	t.Helper()
	parser, err := kong.New(input, kong.Vars{
		"defaultMaxSimilarityCandidates":        "0",
		"defaultMinSimilarityScoreRatio":        "0",
		"defaultMaxSimilarityPhrases":           "0",
		"defaultCommonTextRankEntryRatio":       "0",
		"defaultMinCommonTextRankAbsoluteCount": "0",
		"defaultPruneChangeTriggerRatio":        "0",
	})
	if err != nil {
		t.Fatal(err)
	}
	return parser
}
