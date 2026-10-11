package metadata

import (
	"testing"
)

func TestParseAuthorList(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "single author",
			input: "Jane Austen",
			want:  []string{"Jane Austen"},
		},
		{
			name:  "ampersand separated",
			input: "Neil Gaiman & Terry Pratchett",
			want:  []string{"Neil Gaiman", "Terry Pratchett"},
		},
		{
			name:  "comma separated",
			input: "John Doe, Jane Doe",
			want:  []string{"John Doe", "Jane Doe"},
		},
		{
			name:  "semicolon separated",
			input: "Alice; Bob",
			want:  []string{"Alice", "Bob"},
		},
		{
			name:  "mixed separators",
			input: "Alice & Bob, Carol; Dave",
			want:  []string{"Alice", "Bob", "Carol", "Dave"},
		},
		{
			name:  "empty string",
			input: "",
			want:  nil,
		},
		{
			name:  "only whitespace and separators",
			input: "  , & ;  ",
			want:  nil,
		},
		{
			name:  "leading and trailing whitespace in names",
			input: "  Alice  &  Bob  ",
			want:  []string{"Alice", "Bob"},
		},
		{
			name:  "trailing separator",
			input: "Alice & Bob,",
			want:  []string{"Alice", "Bob"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseAuthorList(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("expected %d names, got %d (%v)", len(tc.want), len(got), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("name[%d]: expected %q, got %q", i, tc.want[i], got[i])
				}
			}
		})
	}
}
