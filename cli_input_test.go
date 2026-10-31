package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

func TestBiblioreadsInstances(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  string
		want []string
	}{
		{name: "defaults"},
		{name: "single", args: []string{"--biblioreads-instances=https://br.example.org"}, want: []string{"https://br.example.org"}},
		{name: "comma separated", args: []string{"--biblioreads-instances=https://br.example.org,http://localhost:8080/"}, want: []string{"https://br.example.org", "http://localhost:8080/"}},
		{name: "repeated", args: []string{"--biblioreads-instances=https://br.example.org", "--biblioreads-instances=https://br2.example.org"}, want: []string{"https://br.example.org", "https://br2.example.org"}},
		{name: "environment", env: "https://br.example.org,https://br2.example.org", want: []string{"https://br.example.org", "https://br2.example.org"}},
		{name: "flag overrides environment", env: "https://env.example.org", args: []string{"--biblioreads-instances=https://cli.example.org"}, want: []string{"https://cli.example.org"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BIBLIOREADS_INSTANCES", tt.env)
			var input CLIInput
			parser := newCLIParser(t, &input)
			if _, err := parser.Parse(append(tt.args, t.TempDir())); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(input.BiblioreadsInstances, tt.want) {
				t.Errorf("instances = %v, want %v", input.BiblioreadsInstances, tt.want)
			}
		})
	}
}

func TestBiblioreadsInstancesRejectInvalidURLs(t *testing.T) {
	for _, instance := range []string{"br.example.org", "/relative", "ftp://br.example.org", "https://", "https://bad host", "https://br.example.org,invalid"} {
		t.Run(instance, func(t *testing.T) {
			var input CLIInput
			parser := newCLIParser(t, &input)
			_, err := parser.Parse([]string{"--biblioreads-instances=" + instance, t.TempDir()})
			if err == nil || !strings.Contains(err.Error(), "biblioreads-instances") {
				t.Fatalf("expected biblioreads-instances validation error, got %v", err)
			}
		})
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
