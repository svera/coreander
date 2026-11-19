package wikidata

import "testing"

func TestNormalizeAuthorID(t *testing.T) {
	tests := []struct {
		input string
		want  string
		valid bool
	}{
		{"", "", true},
		{" \t ", "", true},
		{" Q1234 ", "Q1234", true},
		{"q001", "Q001", true},
		{"https://www.wikidata.org/wiki/Q1234", "Q1234", true},
		{"http://wikidata.org/entity/q1234/", "Q1234", true},
		{"https://www.wikidata.org/wiki/Q1234/extra", "", false},
		{"https://www.wikidata.org/wiki/Q1234//", "", false},
		{"https://www.wikidata.org/wiki/Q1234?x=1", "", false},
		{"https://www.wikidata.org/wiki/Q1234?", "", false},
		{"https://www.wikidata.org/wiki/Q1234#section", "", false},
		{"https://www.wikidata.org/wiki/Q1234#", "", false},
		{"https://user@www.wikidata.org/wiki/Q1234", "", false},
		{"https://www.wikidata.org.evil.example/wiki/Q1234", "", false},
		{"https://www.wikidata.org:443/wiki/Q1234", "", false},
		{"https://www.wikidata.org/wiki/%511234", "", false},
		{"https://www.wikidata.org/wiki/P1234", "", false},
		{"https://www.wikidata.org/w/index.php/Q1234", "", false},
		{"ftp://www.wikidata.org/wiki/Q1234", "", false},
		{"Q", "", false},
		{"Q12a", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := (WikidataSource{}).NormalizeAuthorID(tt.input)
			if (err == nil) != tt.valid || got != tt.want {
				t.Fatalf("NormalizeAuthorID(%q) = %q, %v; want %q, valid=%v", tt.input, got, err, tt.want, tt.valid)
			}
		})
	}
}
