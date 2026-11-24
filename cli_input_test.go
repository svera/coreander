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
		name    string
		args    []string
		ttl     string
		wantTTL int
	}{
		{name: "defaults", wantTTL: 86400},
		{name: "flags", args: []string{"--review-source-cache-ttl=120"}, wantTTL: 120},
		{name: "environment", ttl: "300", wantTTL: 300},
		{name: "flags override environment", ttl: "300", args: []string{"--review-source-cache-ttl=0"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("REVIEW_SOURCE_CACHE_TTL", tt.ttl)
			for key, value := range map[string]string{
				"REVIEW_SOURCE_CACHE_TTL": tt.ttl,
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
			if input.ReviewSourceCacheTTL != tt.wantTTL {
				t.Fatalf("TTL = %d, want %d", input.ReviewSourceCacheTTL, tt.wantTTL)
			}
		})
	}
}

func TestDisableReviewsFlagRemoved(t *testing.T) {
	var input CLIInput
	parser := newCLIParser(t, &input)
	if _, err := parser.Parse([]string{"--disable-reviews", t.TempDir()}); err == nil ||
		!strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("expected removed flag to be rejected, got %v", err)
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
		"defaultMaxSimilarityPhraseFrequency":   "0.01",
		"defaultMinSimilaritySharedPhrases":     "2",
		"defaultMaxSimilaritySubjectFrequency":  "0.2",
		"defaultCommonTextRankEntryRatio":       "0",
		"defaultMinCommonTextRankAbsoluteCount": "0",
		"defaultPruneChangeTriggerRatio":        "0",
	})
	if err != nil {
		t.Fatal(err)
	}

	return parser
}

func TestSimilarityEvidenceSettings(t *testing.T) {
	for _, tc := range []struct {
		name            string
		args            []string
		env             []string
		phrase, subject float64
		minimum         int
	}{
		{name: "defaults", phrase: 0.01, subject: 0.2, minimum: 2},
		{name: "flags", args: []string{"--max-similarity-phrase-frequency=0.05", "--max-similarity-subject-frequency=0.4", "--min-similarity-shared-phrases=3"}, phrase: 0.05, subject: 0.4, minimum: 3},
		{name: "environment", env: []string{"0.1", "0.5", "1"}, phrase: 0.1, subject: 0.5, minimum: 1},
		{name: "flags override environment", env: []string{"0.1", "0.5", "1"}, args: []string{"--max-similarity-phrase-frequency=1", "--max-similarity-subject-frequency=1", "--min-similarity-shared-phrases=4"}, phrase: 1, subject: 1, minimum: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for i, key := range []string{"MAX_SIMILARITY_PHRASE_FREQUENCY", "MAX_SIMILARITY_SUBJECT_FREQUENCY", "MIN_SIMILARITY_SHARED_PHRASES"} {
				t.Setenv(key, "")
				if len(tc.env) > 0 {
					t.Setenv(key, tc.env[i])
				} else if err := os.Unsetenv(key); err != nil {
					t.Fatal(err)
				}
			}
			var input CLIInput
			if _, err := newCLIParser(t, &input).Parse(append(tc.args, t.TempDir())); err != nil {
				t.Fatal(err)
			}
			if input.MaxSimilarityPhraseFrequency != tc.phrase || input.MaxSimilaritySubjectFrequency != tc.subject || input.MinSimilaritySharedPhrases != tc.minimum {
				t.Fatalf("unexpected similarity settings: %g, %g, %d", input.MaxSimilarityPhraseFrequency, input.MaxSimilaritySubjectFrequency, input.MinSimilaritySharedPhrases)
			}
		})
	}
}

func TestSimilarityEvidenceSettingsRejectInvalidValues(t *testing.T) {
	for _, flag := range []string{"max-similarity-phrase-frequency", "max-similarity-subject-frequency", "min-similarity-shared-phrases"} {
		values := []string{"0", "-1"}
		if flag != "min-similarity-shared-phrases" {
			values = append(values, "1.01", "NaN", "Inf")
		}
		for _, value := range values {
			t.Run(flag+"/"+value, func(t *testing.T) {
				var input CLIInput
				_, err := newCLIParser(t, &input).Parse([]string{"--" + flag + "=" + value, t.TempDir()})
				if err == nil || !strings.Contains(err.Error(), flag) {
					t.Fatalf("expected %s validation error, got %v", flag, err)
				}
			})
		}
	}
}
